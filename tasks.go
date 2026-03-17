package salamoonder

import (
	"fmt"
	"time"
)

var tasksLogger = GetLogger("salamoonder.tasks")

type TaskFieldMapping struct {
	Required map[string]string
	Optional []string
}

var TaskFieldMap = map[string]TaskFieldMapping{
	"KasadaCaptchaSolver":        {Required: map[string]string{"pjs": "pjs_url", "cdOnly": "cd_only"}, Optional: nil},
	"KasadaPayloadSolver":        {Required: map[string]string{"url": "url", "script_content": "script_content"}, Optional: []string{"script_url"}},
	"Twitch_PublicIntegrity":     {Required: map[string]string{"access_token": "access_token", "proxy": "proxy"}, Optional: []string{"device_id", "client_id"}},
	"IncapsulaReese84Solver":     {Required: map[string]string{"website": "website", "submit_payload": "submit_payload"}, Optional: []string{"reese_url", "user_agent"}},
	"IncapsulaUTMVCSolver":       {Required: map[string]string{"website": "website"}, Optional: []string{"user_agent"}},
	"AkamaiWebSensorSolver":      {Required: map[string]string{"url": "url", "abck": "abck", "bmsz": "bmsz", "script": "script", "sensor_url": "sensor_url", "count": "count", "data": "data"}, Optional: []string{"user_agent"}},
	"AkamaiSBSDSolver":           {Required: map[string]string{"url": "url", "cookie": "cookie", "sbsd_url": "sbsd_url", "script": "script"}, Optional: []string{"user_agent"}},
	"DataDomeSliderSolver":       {Required: map[string]string{"captcha_url": "captcha_url", "country_code": "country_code"}, Optional: []string{"user_agent"}},
	"DataDomeInterstitialSolver": {Required: map[string]string{"captcha_url": "captcha_url", "country_code": "country_code"}, Optional: []string{"user_agent"}},
}

type Tasks struct {
	client    *SalamoonderSession
	createURL string
	getURL    string
}

func NewTasks(client *SalamoonderSession) *Tasks {
	return &Tasks{
		client:    client,
		createURL: "https://salamoonder.com/api/createTask",
		getURL:    "https://salamoonder.com/api/getTaskResult",
	}
}

func (t *Tasks) CreateTask(taskType string, kwargs map[string]interface{}) (string, error) {
	task := map[string]interface{}{
		"type": taskType,
	}

	if mapping, ok := TaskFieldMap[taskType]; ok {
		for taskKey, kwargKey := range mapping.Required {
			if v, exists := kwargs[kwargKey]; exists {
				task[taskKey] = v
			}
		}
		for _, key := range mapping.Optional {
			if v, exists := kwargs[key]; exists {
				task[key] = v
			}
		}
	}

	tasksLogger.Info("Creating task of type: %s", taskType)

	data, err := t.client.post(t.createURL, map[string]interface{}{"task": task}, "")
	if err != nil {
		return "", err
	}

	taskID, ok := data["taskId"].(string)
	if !ok {
		return "", &APIError{Message: "taskId not found in response"}
	}

	tasksLogger.Info("Task created with ID: %s", taskID)
	return taskID, nil
}

func (t *Tasks) GetTaskResult(taskID string, interval int) (interface{}, error) {
	if interval <= 0 {
		interval = 1
	}

	tasksLogger.Info("Polling task %s (interval=%ds)", taskID, interval)
	attempts := 0

	for {
		attempts++

		data, err := t.client.post(t.getURL, map[string]interface{}{"taskId": taskID}, "")
		if err != nil {
			return nil, err
		}

		status, _ := data["status"].(string)
		tasksLogger.Debug("Task %s status: %s (attempt %d)", taskID, status, attempts)

		if status == "PENDING" {
			time.Sleep(time.Duration(interval) * time.Second)
			continue
		}

		if status == "ready" {
			tasksLogger.Info("Task %s completed after %d attempts", taskID, attempts)
			return data["solution"], nil
		}

		tasksLogger.Error("Task %s failed with status: %s", taskID, status)
		return nil, &APIError{Message: fmt.Sprintf("Unexpected task status: %s", status)}
	}
}
