package domain

// User 是平台账号中本工具用到的字段。ID 统一为字符串：
// 喜茶为会员信息里的 user_main_id，奈雪为 token 载荷里的 userId。
type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
