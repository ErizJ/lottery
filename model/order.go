package model

import (
	"lottery/utils"
	"os"
	"os/signal"
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
	orderCh = make(chan Order, 10000) // 最高瞬时可以下10000单
	stopCh  = make(chan struct{}, 1)
	writeOrderFinish = false 	// true表示所有订单已经持久化到数据库中了
)


// 保证channel里面的数据都持久化到数据库
func listenSingal() {
	c := make(chan os.Signal, 1)  // os.Signal可以容纳各种操作系统级别的信号
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM) // 注册信号2和15，收到任意一种信号就会发送到chan里。Ctrl+c对应SIGINT信号，但是如果是信号9就会直接结束

	for {
		sig := <- c // 阻塞，直到信号的到来，但是还是可能会被强制结束
		if writeOrderFinish { // 订单消费完才可退出
			utils.Logger.Info("recive signal, exit", zap.String("signal", sig.String()))
			os.Exit(0) // 进程退出
		} else {
			utils.Logger.Info("receive signal, but not exit", zap.String("signal", sig.String()))
		}
	}
}

func init() {
	InitChannel()

	go func() {
		TakeOrder()
		writeOrderFinish = true
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