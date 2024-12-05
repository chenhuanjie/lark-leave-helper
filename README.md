飞书用, 在员工请假之后自动创建请假日程, 撤销请假后自动删除请假日程.

行政说我们花的钱不够多, 用不了低代码平台, 能用低代码平台的同学可以参考 [飞书审批员工请假与日历集成 V2.1](https://bytedance.larkoffice.com/wiki/ClivwSmISimT6Aka0JFcP8GSnAQ), 这个不需要自己部署.

# 应用

首先创建企业内建应用, 之后如下操作:

1. 开启功能

   机器人, 设置标注需要拿 bot_id, 我也不知道为啥, 开就行.

2. 开启权限

    * 访问审批应用 (approval:approval:readonly)
    * 创建或删除请假日程 (calendar:timeoff)
    * 获取用户 userID (contact:user.employee_id:readonly)

3. 事件订阅

    * 请假审批 v1.0 (leave_approval)
    * 审批实例状态变更 v1.0 (approval_instance)

4. 可用范围

   不用改, 只有开发者可用就行, 不耽误. 不然由于开了机器人, 所有人都能看到这个没用的应用.

# 部署

建议使用 docker compose 部署, 配置如下:

```yaml
services:
  leave-helper:
    image: chenhuanjie/lark-leave-helper
    restart: always
    environment:
      APP_ID: <Your App ID>
      APP_SECRET: <Your App Secret>
      ENCRYPT_KEY: <Your Encrypt Key, not requierd>
      VERIFICATION_TOKEN: <Your Verification Token, not required>
      REDIS_URL: <Redis url for time off event id storage>
```
