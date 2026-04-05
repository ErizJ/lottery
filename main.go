package main

import (
	"lottery/model"
	"lottery/routes"
	"lottery/utils"
)



func main() {
	// 初始化日志
	utils.InitLogger()
	defer utils.Logger.Sync()
	// 初始化数据库
	model.InitDB()
	// 初始化路由
	routes.InitRoute()
}