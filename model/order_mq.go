/*
写入mq
*/
package model

import (
	"context"
	"encoding/json"
	"lottery/utils"
	"sync"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

var (
	writer *kafka.Writer


	writeWg	sync.WaitGroup
	closeOnce sync.Once
)

func init() {
	InitMQ()
}

// TODO: 同样需要写到配置文件中
func InitMQ() {
	writer = &kafka.Writer{
		Addr: 			kafka.TCP("192.168.197.133:9092"),
		Topic: 			"order",	
		AllowAutoTopicCreation: true,	// topic不存在时自动创建
	}

	// utils.Logger.Info("create writer to mq")
}


// 订单放入mq
func ProduceOrder(userID uint, giftID uint) {
	order := Order{UserId: userID, GiftId: giftID}
	writeWg.Add(1)

	// 异步写入mq，不阻塞抽奖
	go func() {	
		defer writeWg.Done()
		data, _ := json.Marshal(&order)
		if err := writer.WriteMessages(context.Background(), kafka.Message{Value: data}); err != nil {
			utils.Logger.Error("writer kafka failed", zap.String("err", err.Error()))
		}
	}()
}


// 关闭mq连接，CloseMQ可以被反复调用，注意只能关闭一次
func CloseMQ() {
	closeOnce.Do(func ()  {
		writeWg.Wait()  // 保证mq中的消息都写入了mysql
		writer.Close()
		utils.Logger.Info("stop write mq")
	})
}