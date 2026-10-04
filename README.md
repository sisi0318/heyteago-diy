# heyteago-diy

奶茶杯贴 DIY 工具，支持喜茶、奈雪的茶（页面顶部切换平台）。

### 部署

```bash
docker compose up -d    # 打开 http://localhost:3000
```

## token 获取

- 喜茶：部署后打开页面，在登录面板输入手机号，完成滑块人机验证后接收短信验证码，登录成功即自动获得 bearer token。
- 奈雪的茶：小程序登录依赖微信授权，需手机抓包奈雪点单小程序（`tm-api.pin-dao.cn`），把请求头 `authorization` 的值粘贴到页面，有效期约 120 天。

## 致谢

接口思路参考 [FuQuan233/HeyTea_AutoUpload](https://github.com/FuQuan233/HeyTea_AutoUpload)。
本项目仅供学习测试使用。
