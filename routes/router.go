package routes

import (
	v1 "lottery/api/v1"
	"lottery/middleware"
	"lottery/utils"
	"net/http"

	"github.com/gin-gonic/gin"
)

func InitRoute() {
	gin.SetMode(utils.Conf.Server.AppMode)
	r := gin.New()

	// 上线要设置
	// err := r.SetTrustedProxies()

	// 设置中间件
	r.Use(middleware.GinLogger(utils.Logger))
	r.Use(middleware.Cors())
	r.Use(gin.Recovery())
	// 设置静态文件目录
	r.Static("/img", "./view/img")	
	r.Static("/js", "./view/js")
	r.StaticFile("./favicon.ico", "views/img/dqq.png")
	r.LoadHTMLFiles("view/lottery.html") // 应该是index.html


	r.GET("/", func(ctx *gin.Context) {
		ctx.HTML(http.StatusOK, "lottery.html", nil)
	})


	front := r.Group("api/v1")
	{
		// 每个gin.HandlerFunc都会放到一个goroutine里执行
		front.GET("/gifts", v1.GetAllInvent)
		front.GET("/lucky", v1.Lottery)
	}
	
	err := r.Run(utils.Conf.Server.HttpPort)
	if err != nil {
		panic("server start error")
	}
}