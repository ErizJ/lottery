package model

import (
	"context"
	"fmt"
	"lottery/utils"
	"os"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

var (
	lotteryDB *gorm.DB
	lotteryDBOnce sync.Once 


	lotteryRedis *redis.Client
	lotteryRedisOnce sync.Once
	
)

func InitDB() {
	getLotteryDBConnection()
	getRedisClient()
	
	// 将Mysql中的库存同步到redis
	InitInventory()
}


func createMysqlDB() *gorm.DB {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local", 
		utils.Conf.Mysql.DBUser,
		utils.Conf.Mysql.DBPassword,
		utils.Conf.Mysql.DBHost,
		utils.Conf.Mysql.DBPort,
		utils.Conf.Mysql.DBName,
	)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		PrepareStmt: true,
		// 外键约束
		DisableForeignKeyConstraintWhenMigrating: true,

		// 禁用默认事务(提高运行速度)
		SkipDefaultTransaction: true,

		// 使用单数表名
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
	})


	if err != nil {
		utils.Logger.Error("connect mysql failed", zap.String("errmsg", err.Error()))
		os.Exit(1)
	}


	// 设置连接池
	sqlDB, err := db.DB()
	if err != nil {
		utils.Logger.Error("get connection pool failed: %s", zap.String("errmsg", err.Error()))
		os.Exit(1)
	}

	// 设置连接池中的最大闲置连接数
	sqlDB.SetMaxIdleConns(10)

	// 设置数据库最大连接数
	sqlDB.SetMaxOpenConns(100)

	// 设置连接的最大可复用时间
	sqlDB.SetConnMaxLifetime(10 * time.Second)

	return db
}

// 单例
func getLotteryDBConnection() {
	lotteryDBOnce.Do(func ()  {
		if lotteryDB == nil {
			lotteryDB = createMysqlDB()
		}
	})
}


func createRedisClient(addr, passwd string, db int) *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr: addr,
		Password: passwd,
		DB:	db,
	})


	if err := client.Ping(context.Background()).Err(); err != nil {
		utils.Logger.Fatal("connect to redis failed", zap.String("err", err.Error()))
	} else {
		utils.Logger.Info("connect to redis", zap.Int("db", db))
	}

	return client
}


func getRedisClient()  {
	lotteryRedisOnce.Do(func ()  {
		if lotteryRedis == nil {
			lotteryRedis = createRedisClient(utils.Conf.Redis.Addr, "", utils.Conf.Redis.DB)
		}
	})
}