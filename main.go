package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkcalendar "github.com/larksuite/oapi-sdk-go/v3/service/calendar/v4"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	"github.com/redis/go-redis/v9"
	"log"
	"os"
	"strconv"
	"time"
)

var wsClient *larkws.Client
var reqClient *lark.Client
var redisClient *redis.Client

type LeaveApprovalV1Event struct {
	AppID        string `json:"app_id,omitempty"`        // APP ID
	TenantKey    string `json:"tenant_key,omitempty"`    // 企业标识
	Type         string `json:"type,omitempty"`          // 固定为 leave_approval
	InstanceCode string `json:"instance_code,omitempty"` // 审批实例Code
	EmployeeID   string `json:"employee_id,omitempty"`   // 员工 ID
	OpenID       string `json:"open_id,omitempty"`       // 用户open_id
	StartTime    int64  `json:"start_time,omitempty"`    // 销假单关联的原始单据
	EndTime      int64  `json:"end_time,omitempty"`      // 审批结束时间

	LeaveType      string `json:"leave_type,omitempty"`       // 假期名称
	LeaveUnit      int64  `json:"leave_unit,omitempty"`       // 请假最小时长
	LeaveStartTime string `json:"leave_start_time,omitempty"` // 请假开始时间
	LeaveEndTime   string `json:"leave_end_time,omitempty"`   // 请假结束时间
	LeaveInterval  int64  `json:"leave_interval,omitempty"`   // 请假时长，单位（秒）
	LeaveReason    string `json:"leave_reason,omitempty"`     // 请假事由
}

type LeaveApprovalV1Payload struct {
	*larkevent.EventBase
	Event *LeaveApprovalV1Event `json:"event"`
}

type LeaveApprovalRevertEvent struct {
	AppID        string `json:"app_id,omitempty"`        // APP ID
	TenantKey    string `json:"tenant_key,omitempty"`    // 企业标识
	Type         string `json:"type,omitempty"`          // 固定为 leave_approval_revert
	InstanceCode string `json:"instance_code,omitempty"` // 审批实例Code
	OperateTime  int64  `json:"operate_time,omitempty"`  // 销假单关联的原始单据
}

type LeaveApprovalRevertPayload struct {
	*larkevent.EventBase
	Event *LeaveApprovalRevertEvent `json:"event"`
}

func TimeOffEventIdKey(approvalInstanceId string) string {
	return fmt.Sprintf("leaveHelper:approval:%s", approvalInstanceId)
}

func FormatInvokeError(requestId string, code larkcore.CodeError, data any) error {
	return fmt.Errorf("invoke failed: id=%s, code: %s, data: %+v", requestId, larkcore.Prettify(code), data)
}

func CreateTimeOffEvent(ctx context.Context, userId string, start, end time.Time) (string, error) {
	timeOffEvent := larkcalendar.NewTimeoffEventBuilder().
		UserId(userId).
		Timezone("Asia/Shanghai").
		StartTime(strconv.FormatInt(start.Unix(), 10)).
		EndTime(strconv.FormatInt(end.Unix(), 10)).
		Build()
	req := larkcalendar.NewCreateTimeoffEventReqBuilder().TimeoffEvent(timeOffEvent).UserIdType("user_id").Build()
	rsp, err := reqClient.Calendar.TimeoffEvent.Create(ctx, req)
	if err != nil {
		return "", err
	}
	if !rsp.Success() {
		return "", FormatInvokeError(rsp.RequestId(), rsp.CodeError, rsp.Data)
	}
	return *rsp.Data.TimeoffEventId, nil
}

func DeleteTimeOffEvent(ctx context.Context, timeOffEventId string) error {
	req := larkcalendar.NewDeleteTimeoffEventReqBuilder().TimeoffEventId(timeOffEventId).Build()
	rsp, err := reqClient.Calendar.TimeoffEvent.Delete(ctx, req)
	if err != nil {
		return err
	}
	if !rsp.Success() {
		return FormatInvokeError(rsp.RequestId(), rsp.CodeError, nil)
	}
	return nil
}

func OnLeaveApprovalV1(ctx context.Context, event *larkevent.EventReq) error {
	var payload LeaveApprovalV1Payload
	err := json.Unmarshal(event.Body, &payload)
	if err != nil {
		log.Printf("OnLeaveApprovalV1 json Unmarshal error: %+v", err)
		return nil
	}

	// 创建请假日程
	timeFmt := "2006-01-02 15:04:05"
	loc, _ := time.LoadLocation("Asia/Shanghai")
	start, errStart := time.ParseInLocation(timeFmt, payload.Event.LeaveStartTime, loc)
	end, errEnd := time.ParseInLocation(timeFmt, payload.Event.LeaveEndTime, loc)
	if errStart != nil || errEnd != nil {
		log.Printf("OnLeaveApprovalV1 time Parse error: %+v, %+v", errStart, errEnd)
		return nil
	}
	timeOffEventId, err := CreateTimeOffEvent(ctx, payload.Event.EmployeeID, start, end)
	if err != nil {
		log.Printf("OnLeaveApprovalV1 CreateTimeOffEvent error: %+v", err)
		return nil
	}

	now := time.Now()
	if end.After(now) {
		key := TimeOffEventIdKey(payload.Event.InstanceCode)
		err = redisClient.SetEx(ctx, key, timeOffEventId, end.Sub(now)).Err()
		if err != nil {
			log.Printf("OnLeaveApprovalV1 save timeOffEventId error: %+v", err)
		}
	}
	return nil
}

func OnLeaveApprovalV2(ctx context.Context, event *larkevent.EventReq) error {
	// 不处理 leave_approvalV2
	return nil
}

func OnLeaveApprovalRevert(ctx context.Context, event *larkevent.EventReq) error {
	var payload LeaveApprovalRevertPayload
	err := json.Unmarshal(event.Body, &payload)
	if err != nil {
		log.Printf("OnLeaveApprovalRevert json Unmarshal error: %+v", err)
		return nil
	}

	// 查询请假日程ID
	key := TimeOffEventIdKey(payload.Event.InstanceCode)
	timeOffEventId, err := redisClient.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		log.Printf("OnLeaveApprovalRevert timeOffEventId not found")
		return nil
	}

	err = DeleteTimeOffEvent(ctx, timeOffEventId)
	if err != nil {
		log.Printf("OnLeaveApprovalRevert DeleteTimeOffEvent error: %+v", err)
	}
	return nil
}

func main() {
	appId := os.Getenv("APP_ID")
	appSecret := os.Getenv("APP_SECRET")
	if appId == "" || appSecret == "" {
		log.Fatal("APP_ID or APP_SECRET is not set")
	}
	verificationToken := os.Getenv("VERIFICATION_TOKEN")
	encryptKey := os.Getenv("ENCRYPT_KEY")

	redisUrl := os.Getenv("REDIS_URL")
	if redisUrl == "" {
		log.Fatal("REDIS_URL is not set")
	}
	redisOpt, err := redis.ParseURL(redisUrl)
	if err != nil {
		log.Fatalf("bad redis url: %s", redisUrl)
	}
	redisClient = redis.NewClient(redisOpt)
	err = redisClient.Ping(context.Background()).Err()
	if err != nil {
		log.Fatalf("redis ping error: %s", err.Error())
	}

	handler := dispatcher.NewEventDispatcher(verificationToken, encryptKey)
	handler.OnCustomizedEvent("leave_approval", OnLeaveApprovalV1)
	handler.OnCustomizedEvent("leave_approvalV2", OnLeaveApprovalV2)
	handler.OnCustomizedEvent("leave_approval_revert", OnLeaveApprovalRevert)

	wsClient = larkws.NewClient(appId, appSecret, larkws.WithEventHandler(handler), larkws.WithLogLevel(larkcore.LogLevelInfo))
	reqClient = lark.NewClient(appId, appSecret)
	err = wsClient.Start(context.Background())
	if err != nil {
		log.Fatalf("client start error: %+v", err)
	}
}
