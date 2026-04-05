package model

import (
	"context"
	"lottery/utils"
	"lottery/utils/errmsg"
	"strconv"

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
func GetAllInventoryCount() []*Inventory {
	ctx := context.Background()
	// redis key是prefix+id，value是count
	keys, err := lotteryRedis.Keys(ctx, prefix + "*").Result() // 根据前缀获取所有奖品的Key
	if err != nil {
		utils.Logger.Error("iterate all keys by prefix failed", zap.String("prefix", prefix), zap.String("err", err.Error()))
		return nil 
	}

	inventories := make([]*Inventory, 0, len(keys))
	for _, key := range keys { // 根据奖品Key获得奖品的库存count
		if id, err := strconv.Atoi(key[len(prefix):]); err != nil {
			utils.Logger.Error("invalid redis key", zap.String("key", key))
		} else {
			count, err := lotteryRedis.Get(ctx, key).Int() // 根据key从redis中获取库存
			if err != nil {
				utils.Logger.Error("invalid inventory", zap.String("errmsg", err.Error()))
			}
			inventories = append(inventories, &Inventory{ID: uint(id), Count: count})
		}

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


// DeleteInventory 奖品库存-1
func DeleteInventory(invId uint) int {
	ctx := context.Background()
	n, err := lotteryRedis.Decr(ctx, prefix + strconv.Itoa(int(invId))).Result()	// 返回删除后的库存

	if err != nil {
		utils.Logger.Error("delete inventory failed", zap.String("errmsg", err.Error()))

		return errmsg.ERROR
	}

	if n < 0 {
		utils.Logger.Error("the count of inventory is empty, operation failed", zap.Uint("id", invId))
		return errmsg.ERROR
	}

	return errmsg.SUCCESS
}