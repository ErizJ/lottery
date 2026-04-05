package model

import (
	"context"
	"lottery/utils"
	"lottery/utils/errmsg"
	"strconv"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	prefix = "gift_count_" // 设置redis的key统一前缀，方便按前缀遍历key
	EMPTY_GIFT = 1	// 空奖品，谢谢参与
)

type Inventory struct {
	ID 			uint 	`gorm:"column:id"`
	Name 		string 	`gorm:"column:name"`
	Description string  `gorm:"column:description"`
	Picture 	string 	`gorm:"column:picture"`
	Price 		int 	`gorm:"column:price"`
	Count 		int		`gorm:"column:count"`
}

func (Inventory) TableName() string {
	return "inventory"
}

// InitGiftInventory 从mysql读出所有奖品的初始库存，存入redis。如果同时有很多用户参与抽奖，不能发去Mysql里减库存，Mysql扛不住这么高的并发，Redis可以
func InitInventory() {
	ctx := context.Background()
	inventoryCh := make(chan Inventory, 100)
	go GetAllInventoryV2(inventoryCh)
	for {
		inv, ok := <-inventoryCh
		if !ok {	// 消费完
			break
		}

		if inv.Count <= 0 {
			continue // 没有库存的商品不参与抽奖
		}

		err := lotteryRedis.Set(ctx, prefix+strconv.Itoa(int(inv.ID)), inv.Count, 0).Err() // 0表示不设置过期时间
		if err != nil {
			utils.Logger.Fatal("set inv failed", zap.Uint("inv", inv.ID))
		}
	}
}

// GetAllInventoryCount 获取所有奖品的剩余库存量，返回的结果只包含id和count
// 使用 MGET 批量拉取，将 N 次 RTT 降为 2 次（KEYS + MGET）
func GetAllInventoryCount() []*Inventory {
	ctx := context.Background()
	keys, err := lotteryRedis.Keys(ctx, prefix+"*").Result()
	if err != nil {
		utils.Logger.Error("iterate all keys by prefix failed", zap.String("prefix", prefix), zap.String("err", err.Error()))
		return nil
	}
	if len(keys) == 0 {
		return nil
	}

	vals, err := lotteryRedis.MGet(ctx, keys...).Result()
	if err != nil {
		utils.Logger.Error("mget inventory failed", zap.String("err", err.Error()))
		return nil
	}

	inventories := make([]*Inventory, 0, len(keys))
	for i, key := range keys {
		id, err := strconv.Atoi(key[len(prefix):])
		if err != nil {
			utils.Logger.Error("invalid redis key", zap.String("key", key))
			continue
		}
		if vals[i] == nil {
			continue
		}
		count, err := strconv.Atoi(vals[i].(string))
		if err != nil {
			utils.Logger.Error("invalid inventory value", zap.String("key", key))
			continue
		}
		inventories = append(inventories, &Inventory{ID: uint(id), Count: count})
	}
	return inventories
}

// GetAllInventoryV1 读取全部数据
func GetAllInventoryV1() ([]*Inventory, int, int64) {
	var inventoryList []*Inventory
	var total int64
	err := lotteryDB.Select([]string{"id", "name", "description", "picture", "price", "count"}).Find(&inventoryList).Error
	if err != nil {
		utils.Logger.Info("read table failed")
		return inventoryList, errmsg.ERROR_GIFTS_NOT_EXIST, 0
	}

	lotteryDB.Model(&inventoryList).Count(&total)

	return inventoryList, errmsg.SUCCESS, total
}


// !GetAllInventoryV2 获取全部数据，千万级以上大表遍历方案，简单说每次读取一小份，将这一小份放到channel，另一个Goroutine从channel里读
func GetAllInventoryV2(ch chan <- Inventory) { // 只发送通道
	const pageSize = 500	// 一次读取500条
	maxid := 0 
	for {
		var inventoryList []Inventory
		err := lotteryDB.Select([]string{"id", "name", "description", "picture", "price", "count"}).Where("id>?", maxid).Limit(pageSize).Find(&inventoryList).Error	// 用maxid控制每一次读的起始位置

		if err != nil {
			utils.Logger.Error("get inventory data failed", zap.String("errmsg", err.Error()))
			break
		}

		if len(inventoryList) == 0 {	
			break
		}

		for _, inv := range inventoryList {
			if inv.ID > uint(maxid) {
				maxid = int(inv.ID)
			}

			ch <- inv
		}
	}

	close(ch)
}

// AddInventory	奖品库存+1
func AddInventory(invId uint) int {
	ctx := context.Background()
	key := prefix + strconv.Itoa(int(invId))

	_, err := lotteryRedis.Incr(ctx, key).Result()
	if err != nil {
		utils.Logger.Error("incr key failed", zap.String("key", key))
		return errmsg.ERROR
	}

	return errmsg.SUCCESS
}


// luaDecr 原子地检查库存 > 0 再扣减，避免并发时出现负库存
// 返回 1 表示扣减成功，0 表示库存已空
var luaDecr = redis.NewScript(`
local n = tonumber(redis.call("GET", KEYS[1]))
if n and n > 0 then
    redis.call("DECR", KEYS[1])
    return 1
end
return 0
`)

// DeleteInventory 原子扣减奖品库存 -1，库存不足时返回 ERROR
func DeleteInventory(invId uint) int {
	ctx := context.Background()
	key := prefix + strconv.Itoa(int(invId))
	res, err := luaDecr.Run(ctx, lotteryRedis, []string{key}).Int()
	if err != nil {
		utils.Logger.Error("delete inventory lua failed", zap.String("errmsg", err.Error()))
		return errmsg.ERROR
	}
	if res == 0 {
		utils.Logger.Error("inventory empty, decr rejected", zap.Uint("id", invId))
		return errmsg.ERROR
	}
	return errmsg.SUCCESS
}