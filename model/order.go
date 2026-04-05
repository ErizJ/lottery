package model

import (
	"lottery/utils"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"go.uber.org/zap"
)

type Order struct {
	ID          uint
	GiftId 		uint
	UserId      uint
}


/*
订单缓存在channel里面还是有一定风险的
1. 如果Go进程挂掉(很有可能)，channel里面的数据就没有了
2. 即便有多台服务器，每台服务器一个协程不断向Mysql写数据，Mysql也承受不住这样的写并发量（不能让mysql的负载跟着服务器的增长而增长）
*/
var (
	orderCh          = make(chan Order, 10000) // 最高瞬时可以下10000单
	stopCh           = make(chan struct{}, 1)
	writeOrderFinish int32 // 1 表示所有订单已持久化；用 atomic 避免 data race
)


// listenSingal 保证 channel 里面的数据都持久化到数据库后再退出
func listenSingal() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)

	for {
		sig := <-c
		if atomic.LoadInt32(&writeOrderFinish) == 1 {
			utils.Logger.Info("receive signal, exit", zap.String("signal", sig.String()))
			os.Exit(0)
		} else {
			utils.Logger.Info("receive signal, but not exit", zap.String("signal", sig.String()))
		}
	}
}

func InitOrder() {
	InitChannel()

	go func() {
		TakeOrder()
		atomic.StoreInt32(&writeOrderFinish, 1)
	}()

	go listenSingal()
}

func InitChannel() {
	go func() { // 等待接收关闭订单通道信号
		<-stopCh
		close(orderCh)
	}()
}

// PutOrder 将订单放入channel
func PutOrder(userId, inventoryID uint) {
	order := Order{UserId: userId, GiftId: inventoryID}
	orderCh <- order
}

// TakeOrder 从channel中取出订单，写入Mysql
func TakeOrder() {
	for {
		order, ok := <- orderCh

		if !ok {
			utils.Logger.Info("order channel is closed")
			break
		}

		CreateOrder(order.UserId, order.GiftId) // 写入mysql
	}
}


// CreateOrder 创建订单，写入mysql
func CreateOrder(userId, inventoryId uint) int {
	order := Order{GiftId: inventoryId, UserId: userId}
	if err := lotteryDB.Create(&order).Error; err != nil {
		utils.Logger.Error("create order failed", zap.String("errmsg", err.Error()))
		return 0
	} else {
		utils.Logger.Info("create order", zap.Uint("id", order.ID))
		return int(order.ID)
	}
}


// ClearOrders 清除全部订单
func ClearOrders() error {
	err := lotteryDB.Where("id > 0").Delete(Order{}).Error

	if err != nil {
		utils.Logger.Error("delete order failed", zap.String("errmsg", err.Error()))
	}

	return err
}


// CloseChannel 关闭orderCh，通过是使用另一个channel去控制order channel
func CloseChannel() {
	// 第一个发现所有奖品库存为0的向stopCh发送信号，其余的不执行任何操作直接退出
	select {
	case stopCh <- struct{}{}: // 不让函数阻塞再本行代码，外套一个select
	default:	
	}
}