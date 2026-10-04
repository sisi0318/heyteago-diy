package nayukiapi

import (
	"context"
	"os"
	"testing"
	"time"
)

// 真实打奈雪接口的集成测试：默认跳过，NAYUKI_TEST_TOKEN=<抓包 token> 时启用。
// 只调账户信息与取 OSS 凭证两个只读接口，验证签名、请求头与 token 被服务端接受；
// 不上传图片、不提交作品。
func TestLiveReadOnlyEndpoints(t *testing.T) {
	token := os.Getenv("NAYUKI_TEST_TOKEN")
	if token == "" {
		t.Skip("set NAYUKI_TEST_TOKEN to run against the live API")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := New()

	nickname, err := c.Nickname(ctx, token)
	if err != nil {
		t.Fatalf("查询账户信息失败: %v", err)
	}
	t.Logf("账户昵称：%s", nickname)

	creds, err := c.assumeRole(ctx, token)
	if err != nil {
		t.Fatalf("取 OSS 凭证失败: %v", err)
	}
	t.Logf("凭证有效，bucketUrl=%s accessKeyId=%.8s…", creds.BucketURL, creds.AccessKeyID)
}
