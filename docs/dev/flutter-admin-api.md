# Done Hub 管理端 Flutter 对接文档（基于当前 Web 前端实现反向梳理）

> 目标：让你可以按 Web 管理台的能力，快速做一个 Flutter 手机管理客户端。  
> 范围：优先覆盖 **管理端页面已使用的 API**；不深入外部 LLM 转发（`/v1/*`、`/relay/*` 这类）等前端管理面未直接依赖的接口。

---

## 1. 项目后端能力总览（和移动端有关的部分）

后端是 Go + Gin，管理 API 统一在 `/api/*`，路由集中在 `router/api-router.go`。按权限分为：

- 公开接口（状态、公告、价格等）
- 普通用户接口（`middleware.UserAuth()`）
- 管理员接口（`middleware.AdminAuth()`）
- Root 接口（`middleware.RootAuth()`，主要是系统配置）

你要做管理端 App，重点是：

1. 登录态建立（`/api/user/login`）
2. 拉取当前用户（`/api/user/self`）判断角色
3. 调用管理 API（用户、渠道、价格、日志、支付、统计、系统设置等）

---

## 2. 鉴权与会话机制（Flutter 必须先打通）

## 2.1 Web 端当前行为

- 前端统一用 Axios 实例 `API` 调 `/api/*`。`baseURL` 来自 `VITE_APP_SERVER` 或 `/`。
- 登录成功后，前端马上调用 `/api/user/self` 获取用户信息。
- 401 会清空本地用户态并回到登录流程。

## 2.2 Flutter 端建议

### 推荐实现

- 用 `dio`，开启 Cookie 持久化（`cookie_jar` + `dio_cookie_manager`）。
- 请求基地址设置为服务端地址（如 `https://your-domain.com`）。
- API 前缀统一 `/api`。

### 关键点

- 该项目 Web 主要依赖 **会话态**（通常是 cookie/session）。
- Flutter 若跨域/跨端调用，务必确保 cookie 被保存并随请求发送。
- 先登录，再调用 `/api/user/self`；若 `user.role >= 10` 视为管理员。

---

## 3. 通用响应约定（按 Web 用法总结）

绝大多数管理 API 返回结构可按以下解析：

```json
{
  "success": true,
  "message": "",
  "data": {}
}
```

常见模式：

- 列表页：`data.total_count` + `data.data`（数组）
- 详情页：`data` 为对象
- 操作类（增删改/状态切换）：看 `success` 与 `message`

错误处理：

- 401：会话失效，跳登录
- 429：限流
- 500：服务端异常

---

## 4. 管理端核心 API 地图（按模块）

> 下表优先列出 Web 管理面实际调用到的接口，便于你原样迁移。

## 4.1 登录与身份

- `POST /api/user/login`：账号密码登录
- `GET /api/user/self`：获取当前用户资料（用于判定管理员）
- `GET /api/user/logout`：登出

补充（移动端可按需保留）：

- `GET /api/oauth/*`：第三方 OAuth
- `POST /api/webauthn/*`：WebAuthn（移动端通常不做第一期）

---

## 4.2 仪表盘/概览（管理面首页）

- `GET /api/user/dashboard`
- `GET /api/user/dashboard/rate`
- `GET /api/status`
- `GET /api/notice`
- `GET /api/available_model`
- `GET /api/model_ownedby`

移动端建议：

- 首屏并发拉取上面 4~6 个接口
- 失败可局部重试，不阻塞全页面

---

## 4.3 用户管理（/panel/user）

- `GET /api/user/`：分页列表（常见参数：`page,size,keyword,order`）
- `GET /api/user/:id`：用户详情
- `POST /api/user/`：创建用户
- `PUT /api/user/`：编辑用户
- `DELETE /api/user/:id`：删除用户
- `POST /api/user/manage`：快捷动作（启用/禁用、升降级、删除）
- `POST /api/user/quota/:id`：调整额度
- `GET /api/group/`：获取用户组（用于用户表单下拉）

---

## 4.4 渠道管理（/panel/channel）

- `GET /api/channel/`：渠道列表（分页/筛选）
- `GET /api/channel/:id`：渠道详情
- `POST /api/channel/`：新增渠道
- `PUT /api/channel/`：更新渠道
- `DELETE /api/channel/:id`：删除渠道
- `DELETE /api/channel/batch`：批量删除
- `DELETE /api/channel/disabled`：清理失效渠道
- `GET /api/channel/models`：渠道模型汇总
- `POST /api/channel/provider_models_list`：按 provider 拉模型列表
- `GET /api/channel/test`：全量测试
- `GET /api/channel/test/:id`：单渠道测试
- `GET /api/channel/update_balance`：更新全部余额
- `GET /api/channel/update_balance/:id`：更新单渠道余额

批处理能力：

- `PUT /api/channel/batch/azure_api`
- `PUT /api/channel/batch/del_model`
- `PUT /api/channel/batch/add_model`
- `PUT /api/channel/batch/add_user_group`

渠道标签（如果你的 App 要做完整管理）：

- `GET /api/channel_tag/_all`
- `GET /api/channel_tag/:tag/list`
- `PUT /api/channel_tag/:tag/status/:status`
- 以及 `PUT/DELETE /api/channel_tag/:tag*` 系列

实时校验（SSE路由分组下）：

- `POST /api/sse/channel/check`

---

## 4.5 令牌管理（/panel/token）

- `GET /api/token/`：令牌列表（常见参数：`page,size,keyword,token_name,status`）
- `GET /api/token/:id`：令牌详情
- `POST /api/token/`：新增令牌
- `PUT /api/token/`：更新令牌（可带 `status_only=true` 做状态改动）
- `DELETE /api/token/:id`：删除令牌
- `GET /api/token/playground`：生成/获取 Playground Token

Token 提交体中，Web 端重点字段（建议原样建模）：

```json
{
  "name": "",
  "remain_quota": 0,
  "expired_time": -1,
  "unlimited_quota": true,
  "group": "",
  "backup_group": "",
  "setting": {
    "heartbeat": {
      "enabled": false,
      "timeout_seconds": 30
    },
    "limits": {
      "limit_model_setting": {
        "enabled": false,
        "models": []
      },
      "limits_ip_setting": {
        "enabled": false,
        "whitelist": []
      }
    }
  }
}
```

---

## 4.6 用户分组管理（/panel/user_group）

- `GET /api/user_group/`
- `GET /api/user_group/:id`
- `POST /api/user_group/`
- `PUT /api/user_group/`
- `PUT /api/user_group/enable/:id`
- `DELETE /api/user_group/:id`

常用字段：`symbol,name,ratio,public,api_rate,promotion,min,max,enable`

并且前端常用：

- `GET /api/user_group_map`（显示分组倍率映射）

---

## 4.7 模型运营配置

### 模型归属（/panel/model_ownedby）

- `GET /api/model_ownedby/`
- `GET /api/model_ownedby/:id`
- `POST /api/model_ownedby/`
- `PUT /api/model_ownedby/`
- `DELETE /api/model_ownedby/:id`

字段常见：`id,name,icon`

### 模型信息（/panel/model_info）

- `GET /api/model_info/`
- `GET /api/model_info/:id`
- `POST /api/model_info/`
- `PUT /api/model_info/`
- `DELETE /api/model_info/:id`

字段常见：`model,name,description,context_length,max_tokens,input_modalities,output_modalities,tags`

### 价格体系（/panel/pricing）

- `GET /api/prices`（公开价格列表，前端管理页也会读）
- `GET /api/prices/model_list`
- `POST /api/prices/single`
- `PUT /api/prices/single/*model`
- `DELETE /api/prices/single/*model`
- `POST /api/prices/multiple`
- `PUT /api/prices/multiple/delete`
- `POST /api/prices/sync?updateMode=...`
- `GET /api/prices/updateService`
- `GET /api/ownedby`

---

## 4.8 日志与统计

## 日志（/panel/log）

- `GET /api/log/`：全站日志列表
- `GET /api/log/stat`：日志统计
- `DELETE /api/log/`：删除历史日志（带目标时间戳）
- `GET /api/log/export`：导出日志

普通用户视角（可选）：

- `GET /api/log/self`
- `GET /api/log/self/stat`
- `GET /api/log/self/export`

## 统计（/panel/analytics + /panel/multi_user_stats）

- `GET /api/analytics/statistics`
- `GET /api/analytics/period`
- `GET /api/analytics/recharge`
- `GET /api/analytics/multi_user_stats`
- `GET /api/analytics/multi_user_stats/export`

---

## 4.9 兑换/支付管理

## 兑换码（/panel/redemption）

- `GET /api/redemption/`
- `GET /api/redemption/:id`
- `POST /api/redemption/`
- `PUT /api/redemption/`
- `DELETE /api/redemption/:id`

## 支付通道（/panel/payment）

- `GET /api/payment/`
- `GET /api/payment/:id`
- `POST /api/payment/`
- `PUT /api/payment/`
- `DELETE /api/payment/:id`

## 订单（/panel/payment 订单页）

- `GET /api/payment/order`

---

## 4.10 系统设置（Root 级，/panel/setting + /panel/telegram + /panel/system_info）

> 这部分很多是 `RootAuth`，你的移动管理端如果使用管理员但不是 Root，可能会遇到权限不足。

### 配置项总表

- `GET /api/option/`
- `PUT /api/option/`

### 邀请码运营

- `GET /api/invite-code/`
- `GET /api/invite-code/generate`
- `POST /api/invite-code/`
- `PUT /api/invite-code/:id`
- `DELETE /api/invite-code/:id`
- `POST /api/invite-code/batch-delete`

### Telegram 菜单/机器人

- `GET /api/option/telegram`
- `POST /api/option/telegram`
- `GET /api/option/telegram/:id`
- `DELETE /api/option/telegram/:id`
- `PUT /api/option/telegram/reload`
- `GET /api/option/telegram/status`

### 发票与系统工具

- `POST /api/option/invoice/gen/:time`
- `POST /api/option/invoice/update/:time`
- `GET /api/option/safe_tools`

### 系统日志（Root）

- `POST /api/system_info/log`
- `POST /api/system_info/log/query`
- （前端还调用了）`POST /api/system_info/log/context`

---

## 5. Flutter 端推荐分层（可直接照抄）

建议最少 4 层：

1. `HttpClient`（Dio + Cookie + 错误拦截）
2. `ApiService`（按模块封装 endpoint）
3. `Repository`（分页、筛选参数拼装）
4. `Controller/ViewModel`（Riverpod/Bloc/GetX 均可）

推荐目录：

```text
lib/
  core/network/
    dio_client.dart
    api_result.dart
  features/
    auth/
    dashboard/
    user_manage/
    channel_manage/
    token_manage/
    pricing/
    setting/
```

---

## 6. 迁移优先级（建议按这个顺序做）

### P0（先保证可用）

- 登录、会话续存、退出
- 仪表盘核心数据
- 用户管理 CRUD
- 渠道列表 + 增改删 + 测试
- Token 管理

### P1（运营闭环）

- 用户分组 / 模型归属 / 模型信息 / 价格配置
- 日志检索与导出
- 统计报表
- 兑换码与支付通道

### P2（高级管理）

- Telegram 菜单管理
- 系统级 option 细分项
- System Info 日志分析

---

## 7. 实战注意事项（移动端最容易踩坑）

1. **Cookie 丢失**：Flutter 默认不一定持久化 cookie；必须上 `cookie_jar`。
2. **权限差异**：Admin 与 Root 接口不要混用；页面上按权限显示。
3. **分页参数**：Web 用 `page` 从 1 开始，移动端注意转换。
4. **排序参数**：Web 常把降序写成 `-字段名`。
5. **批量接口**：很多接口支持批处理，移动端可以先做单条，再补批量。
6. **字段透传**：渠道/价格配置字段多，第一版可直接按 Web JSON 结构透传，降低出错率。

---

## 8. Web 页面到 API 的迁移对照（管理端）

- `/panel/user` → `/api/user/*`, `/api/group/*`
- `/panel/channel` → `/api/channel/*`, `/api/channel_tag/*`, `/api/sse/channel/check`
- `/panel/token` → `/api/token/*`, `/api/available_model`, `/api/model_ownedby`
- `/panel/user_group` → `/api/user_group/*`, `/api/user_group_map`
- `/panel/pricing` → `/api/prices/*`, `/api/ownedby`
- `/panel/model_ownedby` → `/api/model_ownedby/*`
- `/panel/model_info` → `/api/model_info/*`
- `/panel/log` → `/api/log/*`
- `/panel/analytics`, `/panel/multi_user_stats` → `/api/analytics/*`
- `/panel/redemption` → `/api/redemption/*`
- `/panel/payment` → `/api/payment/*`
- `/panel/setting` → `/api/option/*`, `/api/invite-code/*`
- `/panel/telegram` → `/api/option/telegram*`
- `/panel/system_info` → `/api/system_info/*`

---

## 9. 给你一个可直接落地的对接策略

如果你想最快把 Web 管理台迁到 Flutter：

1. 先把“页面 -> API”按上面映射建 `ApiService`。
2. 每个模块先做 **列表+搜索+分页**，再做编辑弹窗/详情。
3. 新增与编辑尽量共享一个表单模型（Web 就是这么做的）。
4. 复杂配置（渠道、价格）先做“JSON 高级编辑器 + 基础表单”双模式。
5. 等功能全了再做 UI 精修和交互优化。

这样可在最短时间内得到一个“可管理生产环境”的移动端后台。

