/*
消费服务单独起一个进程，后面分离出来
*/
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"lottery/model"
	"lottery/utils"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/segmentio/kafka-go"
)

// 消息队列存储在磁盘中
var reader *kafka.Reader

func Init() {
	utils.InitLogger()
	model.InitDB()
	reader = kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{utils.Conf.Kafka.Addr},
		Topic:          utils.Conf.Kafka.Topic,
		StartOffset:    kafka.LastOffset,
		GroupID:        "serialize_order",
		CommitInterval: 1 * time.Second,
	})
	fmt.Println("create reader to mq")
}

func listenSignal() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	sig := <-c
	fmt.Println("signal", sig.String())
	// 关闭 reader 会让 ReadMessage 返回错误，ConsumeOrder 退出循环后进程自然结束
	reader.Close()
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