package v1

import (
	"lottery/model"
	"lottery/utils"
	"lottery/utils/errmsg"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// GetAllGifts 初始化所有奖品信息，用于初始化轮盘
func GetAllInvent(ctx *gin.Context) {
	var code int
	inventories, code, total := model.GetAllInventoryV1()
	
	// 去除敏感信息
	for _, inv := range inventories {
		inv.Count = 0
	}

	ctx.JSON(http.StatusOK, gin.H{
		"status": code,
		"data": inventories,
		"total": total,
		"msg": errmsg.GetErrMsg(code),
	})
	
}

// !Lottery 抽奖，并发执行
func Lottery(ctx *gin.Context) {
	for try := 0; try < 10; try++ { // 最多重试10次
		// 1. 首先获取所有奖品剩余库存
		inventories := model.GetAllInventoryCount()
		// 2. 确保从redis中查到的奖品库存量大于0，因为抽奖算法Lottery不支持抽中概率为0的奖品（过滤）
		ids := make([]uint, 0, len(inventories))	// 过滤后保留的奖品
		probs := make([]float64, 0, len(inventories)) // 奖品对应的概率
		for _, inv := range inventories {
			if inv.Count > 0 {
				ids = append(ids, inv.ID)
				probs = append(probs, float64(inv.Count))
			}
		}

		// 3. 如果所有奖品都被抽空了，则返回"0"，表示奖品抽完
		if len(ids) == 0 {
			// !这里不能直接 close管道因为如果对一个已经关闭的管道close会报错，并且并发场景下是会存在这种情况的
			// model.CloseChannel() // 关闭orderCh
			go model.CloseMQ()	// 关闭写mq的连接
			// TODO: errmsg
			ctx.String(http.StatusOK, strconv.Itoa(0)) // 0 表示所有奖品已经抽完
			return
		}

		// 4. 抽中第Index个奖品则返回id
		index := utils.Lottery(probs)	
		invId := ids[index]
		// 减少库存
		// 5. 减redis的库存(注意并发问题，不过redis的decr是并发安全的)，有可能返回负数（并发减库存），但是不会返回抽奖失败，而是尝试10次，有一次成功就返回，如果10次都抽不到，就返回失败，1
		code := model.DeleteInventory(invId)
		if code == 500 {
			continue // 减库存失败，重试
		}

		// !用户ID写死1
		// model.PutOrder(1, invId) // 订单信息写入channel
		model.ProduceOrder(1, invId)	// 写入mq

		ctx.String(http.StatusOK, strconv.Itoa(int(invId))) // 返回奖品id

		// 抽奖算法是个概率问题，但是10次都不成功是小概率时间，但是不能为了特别小的概率时间做太多处理
		return 
	}
 	//如果10次之后还失败，则返回谢谢参与
	ctx.JSON(http.StatusOK, gin.H{
		"msg": "谢谢参与",
		"data": strconv.Itoa(1),
	})
}