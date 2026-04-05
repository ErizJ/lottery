/*
消费服务单独起一个进程，后面分离出来
*/
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"lottery/model"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/segmentio/kafka-go"
)

// 消息队列存储在磁盘中
var reader *kafka.Reader

// TODO： kafka应该有单独的配置文件
func Init() {
	// utils.InitLogger()
	// model.InitDB()
	// TODO: 这里可以设置日志是什么级别，打到哪里
	reader = kafka.NewReader(kafka.ReaderConfig{
		Brokers: 		[]string{"192.168.197.133:9092"},
		Topic: 			"order",			// 一个主题可以代表一个业务的数据
		StartOffset: 	kafka.LastOffset,	// MQ里面的旧数据不再接收了
		GroupID: 		"serialize_order",	// 如果不指定GroupID，则只能消费到1个partition里面的数据，kafka可能将数据存在多个partition
		CommitInterval: 1 * time.Second,	// 每隔多长时间自动commit一次offset，也就是每消费一次数据就上报一次
	})

	fmt.Println("create reader to mq")
}

func listenSignal() {
	c := make(chan os.Signal, 1) 
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)	// 注册信号
	sig := <- c // 等待信号
	reader.Close()
	fmt.Println("signal", sig.String())
	os.Exit(0) // 进程退出
}


// 从mq里取出订单，把订单写入mysql
func ConsumeOrder() {
	for {
		if message, err := reader.ReadMessage(context.Background()); err != nil {
			fmt.Println("read message from mq failed", err.Error())
			break
		} else {
			var order model.Order
			if err := json.Unmarshal(message.Value, &order); err == nil {
				fmt.Println("message partition", message.Partition)
				model.CreateOrder(order.UserId, order.GiftId)	// 写入mysql
			} else {
				fmt.Println("order info is invalid json formal", string(message.Value))
			}
		}
	}
}

func main() {
	Init()
	go listenSignal()
	ConsumeOrder()
}

// go run ./mq_consumer/