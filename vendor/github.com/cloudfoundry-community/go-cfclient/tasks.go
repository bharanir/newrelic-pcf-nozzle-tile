package cfclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"time"

	"github.com/pkg/errors"
)

// TaskListResponse is the JSON response from the API.
type TaskListResponse struct {
	Pagination struct {
		TotalResults int `json:"total_results"`
		TotalPages   int `json:"total_pages"`
		First        struct {
			Href string `json:"href"`
		} `json:"first"`
		Last struct {
			Href string `json:"href"`
		} `json:"last"`
		Next     interface{} `json:"next"`
		Previous interface{} `json:"previous"`
	} `json:"pagination"`
	Tasks []Task `json:"resources"`
}

// Task is a description of a task element.
type Task struct {
	GUID       string `json:"guid"`
	SequenceID int    `json:"sequence_id"`
	Name       string `json:"name"`
	Command    string `json:"command"`
	State      string `json:"state"`
	MemoryInMb int    `json:"memory_in_mb"`
	DiskInMb   int    `json:"disk_in_mb"`
	Result     struct {
		FailureReason string `json:"failure_reason"`
	} `json:"result"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	DropletGUID string    `json:"droplet_guid"`
	Links       struct {
		Self struct {
			Href string `json:"href"`
		} `json:"self"`
		App struct {
			Href string `json:"href"`
		} `json:"app"`
		Droplet struct {
			Href string `json:"href"`
		} `json:"droplet"`
	} `json:"links"`
}

// TaskRequest is a v3 JSON object as described in:
// http://v3-apidocs.cloudfoundry.org/version/3.0.0/index.html#create-a-task
type TaskRequest struct {
	Command          string `json:"command"`
	Name             string `json:"name"`
	MemoryInMegabyte int    `json:"memory_in_mb"`
	DiskInMegabyte   int    `json:"disk_in_mb"`
	DropletGUID      string `json:"droplet_guid"`
}

func (c *Client) makeTaskListRequest() ([]byte, error) {
	req := c.NewRequest("GET", "/v3/tasks")
	resp, err := c.DoRequest(req)
	if err != nil {
		return nil, errors.Wrap(err, "Error requesting tasks")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, errors.Wrapf(err, "Error requesting tasks: status code not 200, it was %d", resp.StatusCode)
	}
	return ioutil.ReadAll(resp.Body)
}

func parseTaskListRespones(answer []byte) (TaskListResponse, error) {
	var response TaskListResponse
	err := json.Unmarshal(answer, &response)
	if err != nil {
		return response, errors.Wrap(err, "Error unmarshaling response %v")
	}
	return response, nil
}

// ListTasks returns all tasks the user has access to.
func (c *Client) ListTasks() ([]Task, error) {
	body, err := c.makeTaskListRequest()
	if err != nil {
		return nil, errors.Wrap(err, "Error requesting tasks")
	}
	response, err := parseTaskListRespones(body)
	if err != nil {
		return nil, errors.Wrap(err, "Error reading tasks")
	}
	return response.Tasks, nil
}

func createReader(tr TaskRequest) (io.Reader, error) {
	rmap := make(map[string]string)
	rmap["command"] = tr.Command
	if tr.Name != "" {
		rmap["name"] = tr.Name
	}
	// setting droplet GUID causing issues
	if tr.MemoryInMegabyte != 0 {
		rmap["memory_in_mb"] = fmt.Sprintf("%d", tr.MemoryInMegabyte)
	}
	if tr.DiskInMegabyte != 0 {
		rmap["disk_in_mb"] = fmt.Sprintf("%d", tr.DiskInMegabyte)
	}

	bodyReader := bytes.NewBuffer(nil)
	enc := json.NewEncoder(bodyReader)
	if err := enc.Encode(rmap); err != nil {
		return nil, errors.Wrap(err, "Error during encoding task request")
	}
	return bodyReader, nil
}

// CreateTask creates a new task in CF system and returns its structure.
func (c *Client) CreateTask(tr TaskRequest) (task Task, err error) {
	bodyReader, err := createReader(tr)
	if err != nil {
		return task, err
	}

	request := fmt.Sprintf("/v3/apps/%s/tasks", tr.DropletGUID)
	req := c.NewRequestWithBody("POST", request, bodyReader)

	resp, err := c.DoRequest(req)
	if err != nil {
		return task, errors.Wrap(err, "Error creating task")
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return task, errors.Wrap(err, "Error reading task after creation")
	}

	err = json.Unmarshal(body, &task)
	if err != nil {
		return task, errors.Wrap(err, "Error unmarshaling task")
	}
	return task, err
}

// TaskByGuid returns a task structure by requesting it with the tasks GUID.
func (c *Client) TaskByGuid(guid string) (task Task, err error) {
	request := fmt.Sprintf("/v3/tasks/%s", guid)
	req := c.NewRequest("GET", request)

	resp, err := c.DoRequest(req)
	if err != nil {
		return task, errors.Wrap(err, "Error requesting task")
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return task, errors.Wrap(err, "Error reading task")
	}

	err = json.Unmarshal(body, &task)
	if err != nil {
		return task, errors.Wrap(err, "Error unmarshaling task")
	}
	return task, err
}

// TasksByApp retuns task structures which aligned to an app identified by the given guid.
func (c *Client) TasksByApp(guid string) ([]Task, error) {
	request := fmt.Sprintf("/v3/apps/%s/tasks", guid)
	req := c.NewRequest("GET", request)

	resp, err := c.DoRequest(req)
	if err != nil {
		return nil, errors.Wrap(err, "Error requesting task")
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "Error reading tasks")
	}

	response, err := parseTaskListRespones(body)
	if err != nil {
		return nil, errors.Wrap(err, "Error parsing tasks")
	}
	return response.Tasks, nil
}

// TerminateTask cancels a task identified by its GUID.
func (c *Client) TerminateTask(guid string) error {
	req := c.NewRequest("PUT", fmt.Sprintf("/v3/tasks/%s/cancel", guid))
	resp, err := c.DoRequest(req)
	if err != nil {
		return errors.Wrap(err, "Error terminating task")
	}
	defer resp.Body.Close()

	if resp.StatusCode != 202 {
		return errors.Wrapf(err, "Failed terminating task, response status code %d", resp.StatusCode)
	}
	return nil
}
