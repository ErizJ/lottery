package errmsg



const (
	ERROR = 500
	SUCCESS = 200


	// 奖品相关
	ERROR_GIFTS_NOT_EXIST = 1001
)



var codeMsg = map[int]string {
	ERROR: "FAIL",
	SUCCESS: "OK",
	ERROR_GIFTS_NOT_EXIST: "gitfs not exist",
}


func GetErrMsg(code int) string {
	return codeMsg[code]
}