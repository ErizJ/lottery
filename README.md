# 高并发抽奖系统

基于 Go + Redis + Kafka + MySQL 实现的转盘抽奖系统，支持高并发场景下的库存安全扣减与订单异步持久化。

## 技术栈

| 层级 | 技术 |
|------|------|
| Web 框架 | Gin |
| 数据库 | MySQL 8 + GORM |
| 缓存 / 库存 | Redis（Lua 脚本原子扣减） |
| 消息队列 | Kafka（订单异步写入） |
| 日志 | Zap |
| 配置 | TOML |

## 项目结构

```
JL0ttery/
├── api/v1/          # HTTP 处理器（抽奖、奖品列表）
├── config/          # 配置文件目录
│   └── config.toml.example
├── db/              # 数据库建表脚本
│   └── bak.sql
├── log/             # 运行日志（自动生成）
├── middleware/      # Gin 中间件（CORS、日志）
├── model/           # 数据模型 + DB/Redis/MQ 操作
├── mq_consumer/     # Kafka 消费者（独立进程）
├── routes/          # 路由注册
├── utils/           # 工具（配置、日志、抽奖算法）
└── view/            # 前端静态资源
    ├── lottery.html
    ├── img/
    └── js/
        ├── lucky-canvas@1.7.25
        └── mock.js
```

---

## 快速启动

### 方式一：纯前端 Mock（无需任何后端服务）

适合前端开发、UI 调试，**不需要** MySQL / Redis / Kafka。

**前提**：安装 Python 3

```bash
make mock
```

浏览器访问：

```
http://localhost:3000/lottery.html?mock=1
```

> URL 末尾的 `?mock=1` 是激活 Mock 模式的开关，去掉则连接真实后端。
> 默认库存：电脑 ×2、无人机 ×1 等稀有奖品，总计 46 件，抽完自动提示活动结束。
> 端口可自定义：`MOCK_PORT=8080 make mock`

---

### 方式二：完整后端启动

#### 1. 环境依赖

| 服务 | 版本要求 |
|------|---------|
| Go | ≥ 1.22 |
| MySQL | ≥ 8.0 |
| Redis | ≥ 6.0 |
| Kafka | ≥ 2.x |

#### 2. 初始化数据库

```sql
-- 在 MySQL 中执行
source db/bak.sql
```

或直接粘贴 `db/bak.sql` 内容执行，会创建 `lottery` 数据库及 `inventory`、`order` 两张表并写入初始奖品数据。

#### 3. 创建配置文件

```bash
cp config/config.toml.example config/config.toml
```

编辑 `config/config.toml`，填入真实的服务地址和密码：

```toml
[server]
app_mode  = "debug"      # 上线改为 release
http_port = ":3000"

[mysql]
db_host     = "127.0.0.1"
db_port     = "3306"
db_user     = "root"
db_password = "your-password"
db_name     = "lottery"

[redis]
addr = "127.0.0.1:6379"
db   = 0

[kafka]
addr  = "127.0.0.1:9092"
topic = "order"
```

#### 4. 启动主服务

```bash
make run
```

启动后访问：

```
http://localhost:3000
```

#### 5. 启动订单消费者（独立进程）

消费者负责从 Kafka 读取订单并写入 MySQL，需要**单独开一个终端**运行：

```bash
go run ./mq_consumer/
```

> 消费者进程和主服务是独立的。主服务负责接收抽奖请求并将订单投递到 Kafka；消费者负责将订单串行写入 MySQL，起到削峰作用。

---

## 架构说明

```
用户点击抽奖
     │
     ▼
  Gin 接口
     │
     ├─ Redis Lua 原子扣减库存（防超发）
     │
     ├─ 抽奖算法（按库存加权随机）
     │
     └─ 投递订单到 Kafka（异步，不阻塞响应）
                    │
                    ▼
             Kafka Consumer
                    │
                    ▼
              写入 MySQL
```

**核心设计要点：**
- Redis 存储实时库存，用 Lua 脚本保证"检查 + 扣减"原子性，彻底杜绝超发
- Kafka 解耦抽奖响应与数据库写入，MySQL 只承受消费者的串行写压力
- 抽奖概率与剩余库存正相关，库存越多被抽中概率越高

---

## API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/gifts` | 获取全部奖品（用于初始化转盘） |
| GET | `/api/v1/lucky` | 执行一次抽奖，返回中奖奖品 ID（`"0"` 表示奖品已抽完） |

---

## 运行测试

### 并发压测

```bash
# 模拟 100 个用户疯狂点击，统计 QPS 和各奖品中奖数量
go test -v ./api/v1/test/ -run=^TestLottery1$ -count=1
go test -v ./api/v1/test/ -run=^TestLottery2$ -count=1
```

压测前需确保主服务已启动（`make run`）。

### 单元测试

```bash
# 测试抽奖算法正确性
go test -v ./utils/test/ -run=^TestLottery$ -count=1

# 测试二分查找边界
go test -v ./utils/test/ -run=^TestBinarySearch$ -count=1
```

---

## Makefile 命令

```bash
make run      # 启动完整后端服务
make mock     # 启动纯前端 Mock（无需后端，端口 3000）
make build    # 编译 Linux amd64 二进制
make gotool   # 格式化 Go 代码
make clean    # 清理编译产物
```
