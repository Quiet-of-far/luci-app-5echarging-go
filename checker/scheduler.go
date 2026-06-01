package checker

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"luci-app-5echarging-go/api"
	"luci-app-5echarging-go/config"
	"luci-app-5echarging-go/models"
	"luci-app-5echarging-go/notifier"
	"luci-app-5echarging-go/prediction"
	"luci-app-5echarging-go/storage"
)

const (
	alertLowEnergy = "low_energy"
	alertDepletion = "depletion"
)

type Checker struct {
	cfg   *config.Config
	store *storage.Store
}

func New(cfg *config.Config, store *storage.Store) *Checker {
	return &Checker{
		cfg:   cfg,
		store: store,
	}
}

func (c *Checker) QueryRoom(room config.Room) (*models.QueryResult, error) {
	current, err := api.GetCurrentStatus(room.Building, room.Room)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	record := models.ElectricityRecord{
		Building:     room.Building,
		Room:         room.Room,
		RemainingKWh: current.RemainingKWh,
		QueryTime:    now,
		MeterTime:    current.MeterTime,
	}

	if _, err := c.store.InsertRecordIfChanged(record, c.cfg.MaxRecordsPerRoom); err != nil {
		return nil, fmt.Errorf("存储记录失败: %w", err)
	}

	status, err := c.buildStatus(room)
	if err != nil {
		return nil, err
	}

	status.RemainingKWh = record.RemainingKWh
	status.MeterTime = record.MeterTime
	status.QueryTime = record.QueryTime

	log.Printf("[checker] %s 当前剩余电量: %.2f 度", room.Label, status.RemainingKWh)
	return &models.QueryResult{Room: *status}, nil
}

func (c *Checker) CheckRoom(room config.Room) (*models.QueryResult, error) {
	result, err := c.QueryRoom(room)
	if err != nil {
		log.Printf("[checker] 查询 %s 失败: %v", room.Label, err)
		return nil, err
	}

	alerts := c.evaluateAlerts(result.Room)
	if len(alerts) > 0 && c.cfg.Email.Enabled && c.shouldSendAlert(room, alerts, result.Room.QueryTime) {
		summary, body := c.buildAlertMessage(result.Room, alerts)
		if err := c.sendRoomEmail(room, summary, body); err != nil {
			log.Printf("[checker] 发送邮件失败 %s: %v", room.Label, err)
		} else {
			c.markAlertsSent(room, alerts, result.Room.QueryTime)
			log.Printf("[checker] 已发送告警: %s (%s)", room.Label, strings.Join(alerts, ","))
		}
	}

	result.Alerts = alerts
	return result, nil
}

func (c *Checker) QueryAll() []models.QueryResult {
	results := make([]models.QueryResult, 0, len(c.cfg.Rooms))
	for _, room := range c.cfg.Rooms {
		result, err := c.CheckRoom(room)
		if err != nil {
			results = append(results, models.QueryResult{
				Room: models.RoomStatus{
					Label:       room.Label,
					Building:    room.Building,
					Room:        room.Room,
					QueryStatus: err.Error(),
				},
			})
			continue
		}
		results = append(results, *result)
	}
	statuses := make([]models.RoomStatus, 0, len(results))
	for _, result := range results {
		statuses = append(statuses, result.Room)
	}
	c.saveStatusCache(statuses)
	return results
}

func (c *Checker) GetStatuses() []models.RoomStatus {
	statuses := make([]models.RoomStatus, 0, len(c.cfg.Rooms))
	for _, room := range c.cfg.Rooms {
		status, err := c.buildStatus(room)
		if err != nil {
			if err == sql.ErrNoRows {
				statuses = append(statuses, models.RoomStatus{
					Label:       room.Label,
					Building:    room.Building,
					Room:        room.Room,
					QueryStatus: "暂无查询记录",
				})
				continue
			}

			statuses = append(statuses, models.RoomStatus{
				Label:       room.Label,
				Building:    room.Building,
				Room:        room.Room,
				QueryStatus: err.Error(),
			})
			continue
		}
		statuses = append(statuses, *status)
	}
	c.saveStatusCache(statuses)
	return statuses
}

func (c *Checker) TestEmail() error {
	if !c.cfg.Email.Enabled {
		return fmt.Errorf("邮件通知未启用")
	}

	seen := make(map[string]bool)
	recipients := make([]string, 0)
	for _, room := range c.cfg.Rooms {
		for _, recipient := range room.Recipients {
			recipient = strings.TrimSpace(recipient)
			if recipient == "" || seen[recipient] {
				continue
			}
			seen[recipient] = true
			recipients = append(recipients, recipient)
		}
	}

	if len(recipients) == 0 {
		return fmt.Errorf("未配置任何宿舍收件人地址")
	}

	emailCfg := c.cfg.Email
	emailCfg.To = recipients
	return notifier.NewEmailNotifier(emailCfg).Send("[抑疑电止] 测试通知", "这是一条测试邮件，用于确认邮件通知配置已生效。")
}

func (c *Checker) buildStatus(room config.Room) (*models.RoomStatus, error) {
	latest, err := c.store.GetLatestRecord(room.Building, room.Room)
	if err != nil {
		return nil, err
	}

	status := &models.RoomStatus{
		Label:            room.Label,
		Building:         room.Building,
		Room:             room.Room,
		RemainingKWh:     latest.RemainingKWh,
		MeterTime:        latest.MeterTime,
		QueryTime:        latest.QueryTime,
		PredictionStatus: "预测不可用",
	}

	records, err := c.store.GetRecentRecords(room.Building, room.Room, c.cfg.Prediction.SampleCount)
	if err != nil {
		return nil, fmt.Errorf("读取历史记录失败: %w", err)
	}

	pred, err := prediction.Calculate(records, c.cfg.Prediction.CustomDailyConsumption)
	if err != nil {
		status.PredictionStatus = predictionStatusMessage(err)
		return status, nil
	}

	status.PredictionStatus = "ok"
	status.PredictedEmptyTime = &pred.EstimatedEmptyTime
	status.DailyConsumptionKWh = &pred.DailyConsumptionKWh
	return status, nil
}

func predictionStatusMessage(err error) string {
	switch {
	case errors.Is(err, prediction.ErrNoRecords):
		return "预测不可用: 暂无历史记录"
	case errors.Is(err, prediction.ErrInsufficientData):
		return "预测不可用: 有效历史记录不足"
	case errors.Is(err, prediction.ErrNoConsumptionObserved):
		return "预测不可用: 最近样本未观察到电量下降，可能刚充值或抄表数据尚未更新"
	default:
		return fmt.Sprintf("预测不可用: %v", err)
	}
}

func (c *Checker) evaluateAlerts(status models.RoomStatus) []string {
	var alerts []string

	if status.QueryStatus != "" {
		return alerts
	}

	if status.RemainingKWh <= c.cfg.LowEnergyThreshold {
		alerts = append(alerts, alertLowEnergy)
	}

	if status.PredictedEmptyTime != nil && c.cfg.DepletionAlertDays > 0 {
		remaining := status.PredictedEmptyTime.Sub(status.QueryTime)
		if remaining <= time.Duration(c.cfg.DepletionAlertDays)*24*time.Hour {
			alerts = append(alerts, alertDepletion)
		}
	}

	return alerts
}

func (c *Checker) shouldSendAlert(room config.Room, alerts []string, now time.Time) bool {
	if !c.cfg.Email.AlertOncePerDay {
		return true
	}

	state, err := c.loadAlertState()
	if err != nil {
		log.Printf("[checker] 读取告警状态失败，将继续发送本次告警: %v", err)
		return true
	}

	return state[dailyAlertKey(room)] != now.Local().Format("2006-01-02")
}

func (c *Checker) markAlertsSent(room config.Room, alerts []string, now time.Time) {
	if !c.cfg.Email.AlertOncePerDay {
		return
	}

	state, err := c.loadAlertState()
	if err != nil {
		log.Printf("[checker] 读取告警状态失败，无法记录每日告警限制: %v", err)
		state = make(map[string]string)
	}
	state[dailyAlertKey(room)] = now.Local().Format("2006-01-02")
	if err := c.saveAlertState(state); err != nil {
		log.Printf("[checker] 写入告警状态失败: %v", err)
	}
}

func (c *Checker) sendRoomEmail(room config.Room, summary, body string) error {
	if !c.cfg.Email.Enabled {
		return nil
	}
	if len(room.Recipients) == 0 {
		return fmt.Errorf("宿舍未配置收件人地址")
	}

	emailCfg := c.cfg.Email
	emailCfg.To = filterRecipients(room.Recipients)
	return notifier.NewEmailNotifier(emailCfg).Send(summary, body)
}

func filterRecipients(recipients []string) []string {
	filtered := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		recipient = strings.TrimSpace(recipient)
		if recipient != "" {
			filtered = append(filtered, recipient)
		}
	}
	return filtered
}

func dailyAlertKey(room config.Room) string {
	return room.Building + ":" + room.Room
}

func (c *Checker) statusCachePath() string {
	return c.cfg.DBPath + ".status.json"
}

func (c *Checker) alertStatePath() string {
	return c.cfg.DBPath + ".alerts.json"
}

func (c *Checker) saveStatusCache(data any) {
	path := c.statusCachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Printf("[checker] 创建状态缓存目录失败: %v", err)
		return
	}

	payload, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		log.Printf("[checker] 序列化状态缓存失败: %v", err)
		return
	}

	if err := os.WriteFile(path, payload, 0o644); err != nil {
		log.Printf("[checker] 写入状态缓存失败: %v", err)
	}
}

func (c *Checker) loadStatusCache() ([]models.RoomStatus, error) {
	data, err := os.ReadFile(c.statusCachePath())
	if err != nil {
		return nil, err
	}

	var statuses []models.RoomStatus
	if err := json.Unmarshal(data, &statuses); err == nil && !looksLikeWrappedResults(statuses) {
		return statuses, nil
	}

	var queryResults []models.QueryResult
	if err := json.Unmarshal(data, &queryResults); err != nil {
		return nil, err
	}

	statuses = make([]models.RoomStatus, 0, len(queryResults))
	for _, result := range queryResults {
		statuses = append(statuses, result.Room)
	}
	return statuses, nil
}

func (c *Checker) loadAlertState() (map[string]string, error) {
	data, err := os.ReadFile(c.alertStatePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return make(map[string]string), nil
		}
		return nil, err
	}

	state := make(map[string]string)
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return state, nil
}

func (c *Checker) saveAlertState(state map[string]string) error {
	path := c.alertStatePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, payload, 0o644)
}

func looksLikeWrappedResults(statuses []models.RoomStatus) bool {
	if len(statuses) == 0 {
		return false
	}

	for _, status := range statuses {
		if status.Label != "" || status.Building != "" || status.Room != "" || status.RemainingKWh != 0 || status.QueryStatus != "" {
			return false
		}
	}

	return true
}

func (c *Checker) buildAlertMessage(status models.RoomStatus, alerts []string) (string, string) {
	summary := fmt.Sprintf("[电量预警] %s", status.Label)
	reasons := make([]string, 0, len(alerts))
	for _, alert := range alerts {
		switch alert {
		case alertLowEnergy:
			reasons = append(reasons, fmt.Sprintf("剩余电量低于阈值 %.2f 度", c.cfg.LowEnergyThreshold))
		case alertDepletion:
			reasons = append(reasons, fmt.Sprintf("预计将在 %d 天内耗尽", c.cfg.DepletionAlertDays))
		}
	}

	lines := []string{
		fmt.Sprintf("宿舍: %s", status.Label),
		fmt.Sprintf("剩余电量: %.2f 度", status.RemainingKWh),
		fmt.Sprintf("告警原因: %s", strings.Join(reasons, "；")),
	}
	if status.MeterTime != nil {
		lines = append(lines, fmt.Sprintf("抄表时间: %s", status.MeterTime.Format("2006-01-02 15:04:05")))
	}
	if status.PredictedEmptyTime != nil {
		lines = append(lines, fmt.Sprintf("预计耗尽时间: %s", status.PredictedEmptyTime.Format("2006-01-02")))
	}
	if status.DailyConsumptionKWh != nil {
		lines = append(lines, fmt.Sprintf("日均消耗: %.2f 度/天", *status.DailyConsumptionKWh))
	}
	if status.PredictionStatus != "ok" && status.PredictionStatus != "" {
		lines = append(lines, status.PredictionStatus)
	}

	return summary, strings.Join(lines, "\n")
}

func (c *Checker) Run(stop <-chan struct{}) {
	c.QueryAll()

	if len(c.cfg.Schedule.CheckHours) > 0 {
		c.runCheckHoursMode(stop)
	} else {
		c.runIntervalMode(stop)
	}
}

func (c *Checker) runIntervalMode(stop <-chan struct{}) {
	ticker := time.NewTicker(time.Duration(c.cfg.Schedule.IntervalMinutes) * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.QueryAll()
		case <-stop:
			return
		}
	}
}

func (c *Checker) runCheckHoursMode(stop <-chan struct{}) {
	hourSet := make(map[int]bool)
	for _, h := range c.cfg.Schedule.CheckHours {
		hourSet[h] = true
	}

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	lastRunHour := -1
	lastRunDay := -1

	for {
		select {
		case t := <-ticker.C:
			hour := t.Hour()
			day := t.YearDay()
			if hourSet[hour] && !(day == lastRunDay && hour == lastRunHour) {
				c.QueryAll()
				lastRunHour = hour
				lastRunDay = day
			}
		case <-stop:
			return
		}
	}
}
