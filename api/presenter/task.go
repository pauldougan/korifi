package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
)

const (
	tasksBase = "/v3/tasks"
)

type TaskResponse struct {
	Name          string        `json:"name"`
	GUID          string        `json:"guid"`
	Command       string        `json:"command"`
	Relationships Relationships `json:"relationships"`
	Links         TaskLinks     `json:"links"`
	SequenceID    int64         `json:"sequence_id"`
	CreatedAt     string        `json:"created_at"`
	UpdatedAt     string        `json:"updated_at"`
}

type TaskLinks struct {
	Self Link `json:"self"`
	App  Link `json:"app"`
}

func ForTask(responseTask repositories.TaskRecord, baseURL url.URL) TaskResponse {
	creationTimestamp := responseTask.CreationTimestamp.Format("2006-01-02T15:04:05Z")

	return TaskResponse{
		Name:       responseTask.Name,
		GUID:       responseTask.GUID,
		Command:    responseTask.Command,
		SequenceID: responseTask.SequenceID,
		CreatedAt:  creationTimestamp,
		UpdatedAt:  creationTimestamp,
		Relationships: Relationships{
			"app": Relationship{
				Data: &RelationshipData{
					GUID: responseTask.AppGUID,
				},
			},
		},
		Links: TaskLinks{
			Self: Link{
				HREF: buildURL(baseURL).appendPath(tasksBase, responseTask.GUID).build(),
			},
			App: Link{
				HREF: buildURL(baseURL).appendPath(appsBase, responseTask.AppGUID).build(),
			},
		},
	}
}
