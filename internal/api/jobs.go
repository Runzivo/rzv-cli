package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
)

// Job is one GitHub Actions job Runzivo admitted. Status and Conclusion
// must be read together: status "completed" is Runzivo's own hand-off, and
// only conclusion "success" means the workflow job succeeded.
type Job struct {
	JobID            string   `json:"job_id"`
	WorkflowJobID    int64    `json:"workflow_job_id"`
	RunAttempt       int64    `json:"run_attempt"`
	RepositoryID     string   `json:"repository_id"`
	Repository       string   `json:"repository"`
	Workflow         string   `json:"workflow"`
	RunnerLabel      string   `json:"runner_label"`
	Status           string   `json:"status"`
	Conclusion       *string  `json:"conclusion"`
	QueuedAt         string   `json:"queued_at"`
	StartedAt        *string  `json:"started_at"`
	CompletedAt      *string  `json:"completed_at"`
	ScheduledAt      *string  `json:"scheduled_at"`
	VCPUCount        int64    `json:"vcpu_count"`
	MemSizeMiB       int64    `json:"mem_size_mib"`
	BillableMinutes  *float64 `json:"billable_minutes"`
	HostID           *string  `json:"host_id"`
	LeaseIssuedAt    *string  `json:"lease_issued_at"`
	LeaseClaimedAt   *string  `json:"lease_claimed_at"`
	LeaseCompletedAt *string  `json:"lease_completed_at"`
	WaitReason       string   `json:"wait_reason,omitempty"`
}

func (j *Job) valid() bool { return nonEmpty(j.JobID, j.Status) }

// Jobs calls listCustomerJobs: every job, newest first.
func (c *Client) Jobs(ctx context.Context, tenantID string) ([]Job, error) {
	var body struct {
		Jobs *[]Job `json:"jobs"`
	}
	if err := c.do(ctx, http.MethodGet, tenantPath(tenantID, "/jobs"), nil, nil, &body); err != nil {
		return nil, err
	}
	if err := required(body.Jobs != nil); err != nil {
		return nil, err
	}
	for i := range *body.Jobs {
		if err := required((*body.Jobs)[i].valid()); err != nil {
			return nil, err
		}
	}
	return *body.Jobs, nil
}

// Job calls getCustomerJob.
func (c *Client) Job(ctx context.Context, tenantID, jobID string) (*Job, error) {
	var body struct {
		Job *Job `json:"job"`
	}
	if err := c.do(ctx, http.MethodGet, tenantPath(tenantID, "/job"), url.Values{"jobId": {jobID}}, nil, &body); err != nil {
		return nil, err
	}
	if err := required(body.Job != nil && body.Job.valid()); err != nil {
		return nil, err
	}
	return body.Job, nil
}

// Usage is the workspace's all-time running usage total.
type Usage struct {
	PeriodStart     string   `json:"period_start"`
	PeriodEnd       string   `json:"period_end"`
	BillableMinutes float64  `json:"billable_minutes"`
	JobCount        int64    `json:"job_count"`
	Notes           []string `json:"notes"`
}

// UsageOutcome is "recorded" with Usage set, "not_linked" (no GitHub App
// installation linked) or "not_recorded" (linked, no finished job yet).
type UsageOutcome struct {
	Kind  string `json:"kind"`
	Usage *Usage `json:"usage,omitempty"`
}

// Usage calls getCustomerUsage.
func (c *Client) Usage(ctx context.Context, tenantID string) (*UsageOutcome, error) {
	var body struct {
		Usage *Usage `json:"usage"`
	}
	err := c.do(ctx, http.MethodGet, tenantPath(tenantID, "/usage"), nil, nil, &body)
	var apiErr *Error
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
		switch apiErr.Code {
		case "installation_not_linked":
			return &UsageOutcome{Kind: "not_linked"}, nil
		case "usage_not_recorded":
			return &UsageOutcome{Kind: "not_recorded"}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	if err := required(body.Usage != nil && body.Usage.Notes != nil); err != nil {
		return nil, err
	}
	return &UsageOutcome{Kind: "recorded", Usage: body.Usage}, nil
}
