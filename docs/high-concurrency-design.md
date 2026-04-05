# 高并发抽奖系统技术详解

> 本文从面试角度出发，结合项目源码逐层拆解系统的每一个设计决策，重点回答"为什么这么做"以及"这么做解决了什么问题"。

---

## 目录

1. [系统整体架构](#1-系统整体架构)
2. [启动流程](#2-启动流程)
3. [库存初始化：大表游标分页](#3-库存初始化大表游标分页)
4. [抽奖核心算法：加权随机 + 二分查找](#4-抽奖核心算法加权随机--二分查找)
5. [高并发安全：Redis Lua 原子扣减](#5-高并发安全redis-lua-原子扣减)
6. [每次抽奖的完整执行链路](#6-每次抽奖的完整执行链路)
7. [订单异步写入：Kafka 削峰](#7-订单异步写入kafka-削峰)
8. [消费者进程：串行写 MySQL](#8-消费者进程串行写-mysql)
9. [优雅退出](#9-优雅退出)
10. [Channel 方案的设计与取舍](#10-channel-方案的设计与取舍)
11. [数据库连接池配置](#11-数据库连接池配置)
12. [前端防重复点击与 Mock 机制](#12-前端防重复点击与-mock-机制)
13. [高频面试问题 Q&A](#13-高频面试问题-qa)

---

## 1. 系统整体架构

```
                          ┌─────────────────────────────────┐
                          │          用户浏览器               │
                          │   lucky-canvas 转盘 + jQuery      │
                          └──────────────┬──────────────────┘
                                         │ HTTP GET /api/v1/lucky
                                         ▼
                          ┌─────────────────────────────────┐
                          │         Gin HTTP Server          │
                          │   GET /api/v1/gifts              │
                          │   GET /api/v1/lucky  ◄── 核心    │
                          └──┬──────────────────────────────┘
                             │
              ┌──────────────┼──────────────────┐
              │              │                  │
              ▼              ▼                  ▼
      ┌──────────┐   ┌──────────────┐   ┌──────────────┐
      │  MySQL   │   │    Redis     │   │    Kafka     │
      │  奖品表   │   │  实时库存    │   │  订单消息队列 │
      │  订单表   │   │ Lua原子扣减  │   │  异步写入    │
      └──────────┘   └──────────────┘   └──────┬───────┘
                                               │
                                               ▼
                                    ┌──────────────────┐
                                    │  mq_consumer     │
                                    │  (独立进程)       │
                                    │  串行写 MySQL     │
                                    └──────────────────┘
```

**核心思路**：将高并发的读写压力从 MySQL 转移到 Redis，MySQL 只承受消费者的串行写入，从根本上解决了数据库并发瓶颈。

---

## 2. 启动流程

```go
// main.go
func main() {
    utils.InitLogger()       // 1. 初始化 Zap 日志（写文件 + 控制台）
    defer utils.Logger.Sync() // 进程退出前 flush 缓冲
    model.InitDB()            // 2. 初始化所有基础设施
    routes.InitRoute()        // 3. 启动 HTTP Server（阻塞）
}
```

`model.InitDB()` 是启动的核心，它按固定顺序串行完成所有初始化：

```go
// model/db.go
func InitDB() {
    getLotteryDBConnection() // ① 建立 MySQL 连接池（单例）
    getRedisClient()         // ② 建立 Redis 连接（单例 + Ping 验证）
    InitMQ()                 // ③ 创建 Kafka Writer
    InitOrder()              // ④ 启动订单 channel goroutine 和信号监听
    InitInventory()          // ⑤ 从 MySQL 读库存 → 写入 Redis（最后执行）
}
```

**顺序的重要性**：`InitInventory()` 必须在 Redis 连接建立后才能执行，`InitMQ()` 必须在 Logger 初始化后才能打印日志。这就是为什么从 `init()` 函数迁移到 `InitDB()` 显式调用——Go 的 `init()` 跨包执行顺序不能保证依赖满足。

**单例模式**（`sync.Once`）：

```go
var (
    lotteryDB        *gorm.DB
    lotteryDBOnce    sync.Once
    lotteryRedis     *redis.Client
    lotteryRedisOnce sync.Once
)

func getLotteryDBConnection() {
    lotteryDBOnce.Do(func() {
        if lotteryDB == nil {
            lotteryDB = createMysqlDB()
        }
    })
}
```

`sync.Once` 保证在任何并发情况下，连接只被创建一次，避免重复连接导致资源泄漏。

---

## 3. 库存初始化：大表游标分页

启动时需要将 MySQL 中所有奖品的库存量同步到 Redis，系统采用了**游标分页 + Channel 流水线**的方案：

```go
// model/inventory.go
func InitInventory() {
    ctx := context.Background()
    inventoryCh := make(chan Inventory, 100) // 缓冲 channel，生产者和消费者解耦
    go GetAllInventoryV2(inventoryCh)        // 生产者 goroutine：分批读 MySQL
    for {
        inv, ok := <-inventoryCh
        if !ok { break }          // channel 关闭，读完
        if inv.Count <= 0 { continue } // 库存为 0 的奖品不参与抽奖
        lotteryRedis.Set(ctx, prefix+strconv.Itoa(int(inv.ID)), inv.Count, 0)
    }
}
```

**为什么不用 `SELECT *`？**

```go
// model/inventory.go
func GetAllInventoryV2(ch chan<- Inventory) {
    const pageSize = 500
    maxid := 0
    for {
        var inventoryList []Inventory
        lotteryDB.Where("id > ?", maxid).Limit(pageSize).Find(&inventoryList)
        // ...
        for _, inv := range inventoryList {
            if inv.ID > uint(maxid) { maxid = int(inv.ID) }
            ch <- inv
        }
    }
    close(ch)
}
```

这是一个**游标分页**方案，用 `WHERE id > maxid LIMIT 500` 代替 `LIMIT offset, 500`。

| 方案 | 问题 |
|------|------|
| `LIMIT offset, size` | offset 越大，MySQL 需要扫描跳过的行越多，百万级数据时性能急剧下降 |
| `WHERE id > maxid LIMIT size` | 每次只扫描从 maxid 开始的 500 行，O(1) 无论数据量多大，配合主键索引极快 |

**Channel 的作用**：生产者（读 MySQL）和消费者（写 Redis）并发执行，读一批写一批，内存中最多只有 600 条记录（100 缓冲 + 当前批次 500），不会因为一次性加载百万级数据撑爆内存。

**Redis Key 的设计**：

```
gift_count_2  →  1000   (篮球)
gift_count_4  →  200    (电脑)
gift_count_9  →  100    (无人机)
...
```

统一前缀 `gift_count_` 方便后续用 `KEYS gift_count_*` 批量获取所有奖品 key。

---

## 4. 抽奖核心算法：加权随机 + 二分查找

### 4.1 为什么用库存量作为权重？

```go
// api/v1/inventory.go  Lottery()
for _, inv := range inventories {
    if inv.Count > 0 {
        ids   = append(ids, inv.ID)
        probs = append(probs, float64(inv.Count)) // 库存量即概率权重
    }
}
index := utils.Lottery(probs)
```

库存越多的奖品被抽到的概率越高。以初始数据为例：
- 篮球 1000 件、电脑 200 件，则篮球被抽中的概率是电脑的 5 倍
- 随着库存减少，稀有奖品被抽到的概率自然降低
- 这比固定概率更符合实际活动设计

### 4.2 加权随机的实现

```go
// utils/alog.go
func Lottery(probs []float64) int {
    if len(probs) == 0 { return -1 }

    // 第一步：构建累积概率数组
    // 例如 probs=[1000, 200, 100]
    // cumProbs = [1000, 1200, 1300]
    cumProb := 0.0
    cumProbs := make([]float64, len(probs))
    for i, prob := range probs {
        cumProb += prob
        cumProbs[i] = cumProb
    }

    // 第二步：生成 (0, cumProb] 范围内的随机数
    randNum := rand.Float64() * cumProb
    for randNum == 0 { // 排除 0，避免边界偏差
        randNum = rand.Float64() * cumProb
    }

    // 第三步：二分查找随机数落在哪个区间
    return BinarySearch(cumProbs, randNum)
}
```

**图示理解**：

```
probs    = [1000,  200,  100]
cumProbs = [1000, 1200, 1300]  ← 数轴总长 1300

|←──── 1000 ────→|←─ 200 ─→|← 100 →|
0              1000        1200    1300

随机数落在 [0,1000)   → 抽中篮球 (index 0)  概率 ≈ 76.9%
随机数落在 [1000,1200) → 抽中电脑 (index 1)  概率 ≈ 15.4%
随机数落在 [1200,1300] → 抽中无人机 (index 2) 概率 ≈  7.7%
```

### 4.3 二分查找：找第一个 ≥ target 的元素

```go
func BinarySearch(arr []float64, target float64) int {
    left, right := 0, len(arr)
    for left < right {
        if target <= arr[left]  { return left }      // 命中左边界
        if target > arr[right-1] { return right - 1 } // 超出右边界，返回最后一个
        if left == right-1      { return right - 1 } // 两元素区间，target 在 right-1

        mid := (left + right) / 2
        if target < arr[mid]      { right = mid }  // 在左半区间
        else if target == arr[mid] { return mid }  // 精确命中
        else                       { left = mid }  // 在右半区间（注意不是 mid+1）
    }
    return -1
}
```

**为什么 `left = mid` 而不是 `left = mid + 1`**：这里找的是"区间归属"，不是精确匹配。`target > arr[mid]` 说明 target 落在 `(arr[mid], arr[right-1]]` 区间里，mid 本身是左边界，所以 `left = mid` 保留 mid 作为新的左边界继续收缩。

时间复杂度 O(log n)，相比线性扫描在奖品数量多时有明显优势。

---

## 5. 高并发安全：Redis Lua 原子扣减

### 5.1 问题：DECR 后判断负数为什么不行？

假设某奖品只剩 1 件，有 3 个请求同时到达：

```
请求A: DECR gift_count_4  → 返回 0  ✓（扣成功）
请求B: DECR gift_count_4  → 返回 -1 ✗（发现负数，尝试补回，但已经晚了）
请求C: DECR gift_count_4  → 返回 -2 ✗（同上）
```

在请求B、C检测到负数并尝试回滚（`INCR`）期间，其他操作已经继续，导致：
- 原子性破坏：DECR 和 INCR 之间不是原子的
- 窗口期内库存为负，其他读取到负数的请求可能产生错误行为

### 5.2 解决方案：Lua 脚本

Redis 单线程执行 Lua 脚本，整个脚本是原子的，不会被其他命令打断：

```go
// model/inventory.go
var luaDecr = redis.NewScript(`
local n = tonumber(redis.call("GET", KEYS[1]))
if n and n > 0 then
    redis.call("DECR", KEYS[1])
    return 1   -- 扣减成功
end
return 0       -- 库存不足，拒绝
`)

func DeleteInventory(invId uint) int {
    key := prefix + strconv.Itoa(int(invId))
    res, err := luaDecr.Run(ctx, lotteryRedis, []string{key}).Int()
    if res == 0 { return errmsg.ERROR }   // 库存空
    return errmsg.SUCCESS
}
```

**执行时序对比**：

```
[旧方案] 有竞态窗口
请求A:  DECR → 返回0 → 检查n>=0 → 通过          库存: 1→0
请求B:  DECR → 返回-1                             库存: 0→-1
请求B:          → 检查n<0 → 回滚 INCR             库存: -1→0
（问题：B在DECR后、INCR前，C可能也DECR，导致-2再回滚的混乱）

[新方案] Lua 原子
请求A:  [GET→1, DECR] 整体原子 → 返回1            库存: 1→0
请求B:  [GET→0, 不执行DECR] → 返回0              库存: 0（不变）
请求C:  [GET→0, 不执行DECR] → 返回0              库存: 0（不变）
```

**`redis.NewScript` 的优化**：它会在首次执行时将脚本用 `EVALSHA` 缓存到 Redis，后续调用直接传 SHA1 哈希而非完整脚本，减少网络传输量。

### 5.3 批量读取库存：MGET

每次抽奖前需要读取所有奖品的当前库存：

```go
func GetAllInventoryCount() []*Inventory {
    // 第一步：KEYS 获取所有奖品 key（仅启动和奖品很少时调用，生产环境应缓存 key 列表）
    keys, _ := lotteryRedis.Keys(ctx, prefix+"*").Result()

    // 第二步：MGET 一次性批量获取所有值，将 N 次 RTT 压缩为 1 次
    vals, _ := lotteryRedis.MGet(ctx, keys...).Result()

    // 第三步：解析结果
    for i, key := range keys {
        id, _    := strconv.Atoi(key[len(prefix):])
        count, _ := strconv.Atoi(vals[i].(string))
        inventories = append(inventories, &Inventory{ID: uint(id), Count: count})
    }
}
```

| 方案 | 网络往返次数 | 9 个奖品的耗时（假设 RTT=0.5ms）|
|------|-------------|-------------------------------|
| N 次单独 GET | 9 次 | ≈ 4.5ms |
| KEYS + MGET | 2 次 | ≈ 1ms |

---

## 6. 每次抽奖的完整执行链路

```go
// api/v1/inventory.go
func Lottery(ctx *gin.Context) {
    for try := 0; try < 10; try++ {   // 最多重试 10 次
        // Step 1: 从 Redis 批量读取所有奖品实时库存
        inventories := model.GetAllInventoryCount()

        // Step 2: 过滤库存=0 的奖品，构建抽奖权重数组
        ids, probs := []uint{}, []float64{}
        for _, inv := range inventories {
            if inv.Count > 0 {
                ids   = append(ids, inv.ID)
                probs = append(probs, float64(inv.Count))
            }
        }

        // Step 3: 全部抽完
        if len(ids) == 0 {
            go model.CloseMQ()                        // 异步关闭 Kafka Writer
            ctx.String(http.StatusOK, "0")
            return
        }

        // Step 4: 加权随机抽奖，得到下标
        index := utils.Lottery(probs)
        invId := ids[index]

        // Step 5: Lua 原子扣减 Redis 库存
        code := model.DeleteInventory(invId)
        if code == 500 {
            continue   // 扣减失败（被其他并发请求抢先），重试
        }

        // Step 6: 异步写入 Kafka，不阻塞响应
        model.ProduceOrder(1, invId)

        // Step 7: 返回中奖奖品 ID 给前端
        ctx.String(http.StatusOK, strconv.Itoa(int(invId)))
        return
    }

    // 10 次都失败（极小概率），兜底返回
    ctx.JSON(http.StatusOK, gin.H{"msg": "谢谢参与", "data": "1"})
}
```

**为什么需要重试？**

Step 4（读库存）和 Step 5（扣库存）之间有一个短暂的窗口期：读到的某件奖品库存为 5，但在执行扣减时已被其他并发请求抢走归零，Lua 脚本返回 0，此时 `continue` 重新抽奖。

重试最多 10 次是一个工程上的权衡：在库存充足时几乎不会重试；在库存接近耗尽时偶尔重试；超过 10 次说明当前确实抢不到，返回兜底结果。

**时序图**：

```
用户请求
  │
  ├─ Redis KEYS+MGET (读库存快照)
  │       │ ~1ms
  ├─ 本地计算 (加权随机 + 二分) ~0ms
  │
  ├─ Redis Lua EVALSHA (原子扣减)
  │       │ ~0.5ms
  ├─ go ProduceOrder() (异步，不等待)
  │
  └─ 返回奖品 ID 给用户
         总耗时约 2-3ms
```

---

## 7. 订单异步写入：Kafka 削峰

### 7.1 为什么不直接写 MySQL？

假设系统 QPS 为 10000，如果每次抽奖直接写 MySQL：
- MySQL 单机写入 QPS 约 5000-10000（取决于硬件）
- 每次抽奖接口耗时从 2ms 变成 20ms+，响应变慢
- MySQL 成为瓶颈，随服务实例数增多而恶化

### 7.2 实现

```go
// model/order_mq.go
var (
    writer    *kafka.Writer
    writeWg   sync.WaitGroup // 跟踪所有正在进行的异步写入
    closeOnce sync.Once      // 保证 Close 只执行一次
)

func ProduceOrder(userID uint, giftID uint) {
    order := Order{UserId: userID, GiftId: giftID}
    writeWg.Add(1)   // 在启动 goroutine 前计数，避免 goroutine 调度延迟导致计数不准

    go func() {
        defer writeWg.Done()
        data, err := json.Marshal(&order)
        if err != nil {
            utils.Logger.Error("marshal order failed", ...)
            return   // 序列化失败时直接丢弃，不写空消息
        }
        writer.WriteMessages(context.Background(), kafka.Message{Value: data})
    }()
}
```

**`writeWg.Add(1)` 必须在 goroutine 外部**：如果放在 goroutine 内部，主 goroutine 可能在 `writeWg.Add(1)` 执行前就调用 `writeWg.Wait()`，导致 Wait 提前返回。

### 7.3 关闭时等待所有消息发送完毕

```go
func CloseMQ() {
    closeOnce.Do(func() {
        writeWg.Wait()   // 等待所有 ProduceOrder goroutine 完成
        writer.Close()
        utils.Logger.Info("stop write mq")
    })
}
```

当所有奖品抽完时，`Lottery` 接口调用 `go model.CloseMQ()`（注意是 goroutine，不阻塞响应），确保队列中最后几条消息都被发送后再关闭连接。

**`sync.Once` 的作用**：多个并发的 Lottery 请求都可能触发"奖品抽完"的判断，都会调用 `CloseMQ()`，`Once` 保证 `writer.Close()` 只执行一次，避免 double close panic。

---

## 8. 消费者进程：串行写 MySQL

消费者是一个**独立进程**（`go run ./mq_consumer/`），与主服务进程完全分离：

```go
// mq_consumer/main.go
func Init() {
    utils.InitLogger()
    model.InitDB()
    reader = kafka.NewReader(kafka.ReaderConfig{
        Brokers:        []string{utils.Conf.Kafka.Addr},
        Topic:          utils.Conf.Kafka.Topic,
        StartOffset:    kafka.LastOffset,   // 只消费新消息，不重放历史
        GroupID:        "serialize_order",  // 消费者组，支持多分区
        CommitInterval: 1 * time.Second,    // 每秒自动提交 offset
    })
}

func ConsumeOrder() {
    for {
        message, err := reader.ReadMessage(context.Background())
        if err != nil { break }  // reader 关闭时 ReadMessage 返回错误，退出循环

        var order model.Order
        if err := json.Unmarshal(message.Value, &order); err == nil {
            model.CreateOrder(order.UserId, order.GiftId)  // 串行写 MySQL
        }
    }
}
```

**为什么独立进程而不是主服务的 goroutine？**

| 对比维度 | 独立进程 | 主服务 goroutine |
|---------|---------|----------------|
| MySQL 负载 | 消费者串行写，MySQL 压力恒定 | 多服务实例 × 多 goroutine，MySQL 压力随实例数线性增长 |
| 水平扩展 | 主服务可随意扩容，消费者保持单实例 | 扩容主服务同时扩大 MySQL 写压力 |
| 故障隔离 | 消费者崩溃不影响抽奖服务 | 消费者异常影响主服务 |
| 进程崩溃 | 消息在 Kafka 持久化，重启消费者可续消费 | 进程崩溃则 channel 数据丢失 |

**`GroupID` 的作用**：当 Kafka topic 有多个 partition 时，相同 GroupID 的消费者实例会被分配到不同 partition，实现并行消费。目前单消费者实例已足够，`GroupID` 为将来横向扩展预留了接口。

---

## 9. 优雅退出

系统有两套优雅退出机制，分别服务于主进程和消费者进程。

### 9.1 主进程（order.go 的 Channel 方案）

```go
// model/order.go
func listenSingal() {
    c := make(chan os.Signal, 1)
    signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)

    for {
        sig := <-c
        if atomic.LoadInt32(&writeOrderFinish) == 1 {
            utils.Logger.Info("receive signal, exit")
            os.Exit(0)    // 确认所有订单已写完才退出
        } else {
            utils.Logger.Info("receive signal, but not exit")
            // 继续等待，直到 TakeOrder 消费完 channel
        }
    }
}
```

`writeOrderFinish` 使用 `atomic.Int32` 而非普通 `bool`，是因为它被两个 goroutine 同时读写（listenSingal 读，TakeOrder 结束后的 goroutine 写），若用普通变量会触发 Go race detector 报告 data race。

### 9.2 消费者进程（mq_consumer）

```go
func listenSignal() {
    c := make(chan os.Signal, 1)
    signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
    sig := <-c
    fmt.Println("signal", sig.String())
    reader.Close()  // 关闭 reader
    // 不 os.Exit()，让 ConsumeOrder 的 ReadMessage 返回错误，循环自然退出
}

func ConsumeOrder() {
    for {
        message, err := reader.ReadMessage(...)
        if err != nil { break }  // reader.Close() 后此处返回错误，退出
        model.CreateOrder(...)   // 正在处理的消息会完成，不会被强制中断
    }
}

func main() {
    Init()
    go listenSignal()
    ConsumeOrder()  // main goroutine 阻塞于此，ConsumeOrder 返回则进程退出
}
```

**关键点**：收到信号后不立即 `os.Exit()`，而是关闭 `reader`。`ReadMessage` 感知到 reader 关闭后返回错误，`for` 循环 `break`，`ConsumeOrder` 返回，`main()` 返回，进程自然退出。这样保证了**正在被 `CreateOrder` 处理的那条消息一定会写完**，不会因为强制退出而丢失。

---

## 10. Channel 方案的设计与取舍

项目中保留了一套基于 Go Channel 的订单写入方案（`model/order.go`），最终选择了 Kafka 方案，但理解 channel 方案的局限性是面试的重点：

```go
var (
    orderCh = make(chan Order, 10000) // 缓冲 10000 个订单
    stopCh  = make(chan struct{}, 1)
)

// 两个 channel 的关闭设计
func InitChannel() {
    go func() {
        <-stopCh          // 等待关闭信号
        close(orderCh)    // 关闭订单 channel
    }()
}

func CloseChannel() {
    select {
    case stopCh <- struct{}{}: // 发送关闭信号（非阻塞）
    default:                   // 已有信号则跳过，防止阻塞
    }
}
```

**为什么不直接 `close(orderCh)`**？在并发场景下，多个 goroutine 都可能判断到"库存全部为零"并尝试关闭 channel，对已关闭的 channel 再次 `close` 会 panic。通过 `stopCh` 这个中间层，保证 `orderCh` 只被关闭一次。

**Channel 方案的局限**（代码注释中明确指出）：

```go
/*
订单缓存在 channel 里面还是有一定风险的：
1. 如果 Go 进程挂掉，channel 里面的数据就没有了（进程内存 vs Kafka 磁盘持久化）
2. 即便有多台服务器，每台服务器一个协程不断向 MySQL 写数据，
   MySQL 也承受不住这样的写并发量（不能让 MySQL 负载跟着服务器数量增长而增长）
*/
```

这就是最终选择 Kafka 的原因：持久化保证 + 写压力解耦。

---

## 11. 数据库连接池配置

```go
// model/db.go
sqlDB.SetMaxIdleConns(10)             // 最大空闲连接数
sqlDB.SetMaxOpenConns(100)            // 最大打开连接数
sqlDB.SetConnMaxLifetime(5 * time.Minute) // 连接最大复用时间
```

**参数选择的逻辑**：
- `MaxIdleConns(10)`：空闲连接保持 10 个，避免短时间内频繁创建连接（TCP + MySQL 认证的开销）
- `MaxOpenConns(100)`：与 MySQL 的 `max_connections` 配合，一般设为其 60%-80%，留余量给其他服务
- `ConnMaxLifetime(5min)`：原来设的是 10 秒（有 bug），被修正为 5 分钟。过短会导致连接频繁重建，在流量高峰时增加延迟抖动

**GORM 配置优化**：

```go
gorm.Config{
    PrepareStmt:            true,  // 开启预编译语句缓存，相同 SQL 只解析一次
    SkipDefaultTransaction: true,  // 禁用单条语句的自动事务（写操作减少一次 BEGIN/COMMIT RTT）
}
```

---

## 12. 前端防重复点击与 Mock 机制

### 12.1 防重复点击

```javascript
var spinning = false;

start: function() {
    if (spinning) return;
    spinning = true;          // 立即加锁，防止 ajax 返回前再次点击
    $.ajax({
        url: 'api/v1/lucky',
        success: function(giftId) {
            if (giftId === '0') {
                spinning = false; // 抽完了，解锁
                return;
            }
            // spinning 保持 true，直到转盘停止
            myLucky.play();
            setTimeout(function() { myLucky.stop(idx); }, 3000);
        }
    }).fail(function() {
        spinning = false;  // 请求失败，解锁
    });
},
end: function(prize) {
    spinning = false;  // 转盘停止，解锁
}
```

**为什么要在 `start` 里立即加锁**：如果在 `success` 回调里才加锁，从用户点击到 ajax 响应的这段时间（100-200ms）内，用户可以多次点击，每次点击都会发出一个请求，造成多次扣库存。立即加锁确保每次只有一个在途请求。

### 12.2 Mock 拦截机制

```javascript
// view/js/mock.js
;(function($) {
    // 不在 ?mock=1 模式下直接退出，对真实后端完全透明
    if (new URLSearchParams(location.search).get('mock') !== '1') return;

    var _origAjax = $.ajax.bind($);
    $.ajax = function(options) {
        if (options.url.indexOf('api/v1/gifts') !== -1) {
            setTimeout(function() { options.success({ data: mockGifts }); }, 200);
            return { fail: function() { return this; } };
        }
        if (options.url.indexOf('api/v1/lucky') !== -1) {
            setTimeout(function() { options.success(mockLottery()); }, 120);
            return { fail: function() { return this; } };
        }
        return _origAjax.apply(this, arguments); // 其他请求不拦截
    };
})(jQuery);
```

mock.js 始终被加载，但只有 URL 含 `?mock=1` 时才激活，对生产环境**零侵入**。模拟了真实接口的异步延迟（200ms / 120ms），避免测试时因同步响应掩盖真实的时序问题。

---

## 13. 高频面试问题 Q&A

**Q1：为什么用 Redis 存库存而不是 MySQL？**

MySQL 的写入 QPS 一般在几千到一万左右，高并发抽奖活动中大量请求同时打过来，MySQL 无法承受。Redis 单机读写 QPS 可达 10 万级，且单线程模型天然串行，配合 Lua 脚本可以做到原子操作。

**Q2：Redis Lua 脚本解决了什么问题，为什么不用 SETNX 或事务？**

- SETNX（分布式锁）：加锁 → 扣减 → 解锁，三步操作，锁竞争时大量请求等待，QPS 下降严重
- Redis 事务（MULTI/EXEC）：不支持在 EXEC 执行前基于读到的值做条件判断（WATCH 方案复杂且有性能问题）
- Lua 脚本：GET + 判断 + DECR 三步在 Redis 内部原子执行，无网络往返，无锁等待，是最优解

**Q3：Kafka 消费者为什么用独立进程而不是 goroutine？**

主要原因是**控制 MySQL 写压力**。如果主服务和消费者在同一进程，水平扩展主服务时（比如启动 10 个实例），每个实例都有消费者 goroutine 在写 MySQL，MySQL 压力变成 10 倍。独立进程可以只部署 1 个消费者实例，无论主服务扩容多少，MySQL 写入始终是一个进程在串行处理。

**Q4：抽奖为什么要重试最多 10 次？**

Step4（读库存快照）和 Step5（Lua 扣减）之间有时间差，在高并发时可能出现：读到奖品还有库存，但扣减时已被其他请求抢走。此时 Lua 返回 0，重新执行一轮抽奖。10 次是工程上的权衡，正常活动很少超过 3 次，超过 10 次说明几乎所有奖品都在同一时刻被抢光，此时兜底返回是合理的。

**Q5：`GetAllInventoryCount` 中的 `KEYS` 命令在生产环境有什么风险？**

`KEYS` 命令会扫描全量数据，在 key 数量很多时会阻塞 Redis（单线程），导致其他命令排队。生产环境的优化方案：
1. 在内存中缓存所有奖品的 key 列表（启动时建立，活动期间不变）
2. 或使用 `SCAN` 命令代替 `KEYS`（渐进式扫描，不阻塞）
3. 本项目奖品数量固定（约 10 个），`KEYS` 实际影响极小

**Q6：如果 Kafka 消费者挂掉，订单会丢吗？**

不会丢。Kafka 的消费位点（offset）在消费者挂掉时不会前进（`CommitInterval: 1s` 表示最多丢失 1 秒内已消费但未提交的消息）。消费者重启后从上次提交的 offset 继续消费，可能有少量消息重复消费，所以 `CreateOrder` 应该具备幂等性（当前实现未做，生产环境需要基于 userId+giftId+时间窗口去重）。

**Q7：`sync.Once` 和双重检查锁（DCL）有什么区别？**

```go
// 双重检查锁（Go 中不安全，因为内存模型问题）
if lotteryDB == nil {
    mu.Lock()
    if lotteryDB == nil { lotteryDB = createMysqlDB() }
    mu.Unlock()
}

// sync.Once（正确方式）
lotteryDBOnce.Do(func() { lotteryDB = createMysqlDB() })
```

Go 的内存模型不保证在没有同步原语的情况下指针赋值对其他 goroutine 可见，直接使用 DCL 可能看到未完全初始化的对象。`sync.Once` 内部使用了 `atomic` + `Mutex`，保证了可见性和有序性，是 Go 中单例的标准写法。

**Q8：`atomic.Int32` 和 `bool` + `Mutex` 有什么区别，用哪个？**

对于简单的标志位（只有 0/1 两种状态），`atomic.Int32` 比 `Mutex` 更轻量：
- `Mutex`：上下文切换 + 内核态开销，适合保护复杂的临界区
- `atomic`：CPU 级别的原子指令（CAS / LOCK XCHG），无内核切换，适合单个变量的读写

本项目的 `writeOrderFinish` 只需要一个无竞争的标志位，`atomic.StoreInt32` / `atomic.LoadInt32` 是最合适的选择。
