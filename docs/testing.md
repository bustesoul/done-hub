# 测试

默认全量测试：

```sh
go test -count=1 ./...
```

图片测试使用 `common/image/testdata` 固定样本和本地 HTTP 服务；Ali 协议测试复用项目的模拟服务，不需要公网或真实凭据。

通知和存储测试会发送真实消息或上传真实文件，默认明确标记为 `SKIP`。需要运行时，显式启用并指定独立的凭据配置文件，建议用 `-run` 选择目标服务：

```sh
DONE_HUB_RUN_EXTERNAL_TESTS=1 \
DONE_HUB_TEST_CONFIG=/absolute/path/test-config.yaml \
go test -count=1 -v ./common/notify/channel -run '^TestDingTalkSend$'
```

配置键沿用现有的 `notify.dingtalk.*`、`notify.lark.*`、`notify.pushdeer.*`、`notify.telegram.*`、`storage.alioss.*`、`storage.smms.*`、`storage.imgur.*`。配置文件读取失败会使测试失败；目标服务必需凭据缺失时明确跳过该用例。配置值不写入全局业务配置，也不应提交到仓库。

默认全量测试通过不代表真实通知或上传已验证。测试辅助代码另有本地用例验证启用开关、缺少凭据时的隔离、客户端初始化和清理，不产生外部请求。
