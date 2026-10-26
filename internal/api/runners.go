package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
)

// Concurrency is the workspace's concurrent-job limit and how much of it
// is in use. Admission is "open" or "full" ("held" is reserved).
type Concurrency struct {
	Limit     int64   `json:"limit"`
	Used      int64   `json:"used"`
	Remaining int64   `json:"remaining"`
	Admission string  `json:"admission"`
	Reason    *string `json:"reason"`
}

// ConcurrencyOutcome is "configured" with Concurrency set, or "not_linked"
// (no GitHub App installation linked, so there is no limit to report).
type ConcurrencyOutcome struct {
	Kind        string       `json:"kind"`
	Concurrency *Concurrency `json:"concurrency,omitempty"`
}

// Concurrency calls getCustomerConcurrency.
func (c *Client) Concurrency(ctx context.Context, tenantID string) (*ConcurrencyOutcome, error) {
	var body struct {
		Concurrency *Concurrency `json:"concurrency"`
	}
	err := c.do(ctx, http.MethodGet, tenantPath(tenantID, "/concurrency"), nil, nil, &body)
	var apiErr *Error
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound && apiErr.Code == "installation_not_linked" {
		return &ConcurrencyOutcome{Kind: "not_linked"}, nil
	}
	if err != nil {
		return nil, err
	}
	if err := required(body.Concurrency != nil && body.Concurrency.Admission != ""); err != nil {
		return nil, err
	}
	return &ConcurrencyOutcome{Kind: "configured", Concurrency: body.Concurrency}, nil
}

// ActiveJobs are jobs running on a runner, or admitted and waiting for one.
type ActiveJobs struct {
	RunningJobs int64 `json:"running_jobs"`
	WaitingJobs int64 `json:"waiting_jobs"`
}

// InventoryProfile is one runner size, whether the repository can use it
// now, and the workspace's active jobs on it. Label is the runs-on value.
type InventoryProfile struct {
	Label      string  `json:"label"`
	VCPUCount  int64   `json:"vcpu_count"`
	MemSizeMiB int64   `json:"mem_size_mib"`
	Available  bool    `json:"available"`
	Reason     *string `json:"reason"`
	ActiveJobs
}

// RunnerInventory is getCustomerRunnerInventory's result. Unlisted counts
// jobs on a size that is no longer listed, so the counts still add up.
type RunnerInventory struct {
	Profiles []InventoryProfile `json:"profiles"`
	Unlisted ActiveJobs         `json:"unlisted"`
}

// InventoryOutcome is "available" with Inventory set, "not_linked" (no
// GitHub App installation linked) or "not_authorized" (the repository is not
// among the linked installation's authorized repositories).
type InventoryOutcome struct {
	Kind      string           `json:"kind"`
	Inventory *RunnerInventory `json:"runner_inventory,omitempty"`
}

// RunnerInventory calls getCustomerRunnerInventory.
func (c *Client) RunnerInventory(ctx context.Context, tenantID string, repositoryID int64) (*InventoryOutcome, error) {
	var body struct {
		RunnerInventory *RunnerInventory `json:"runner_inventory"`
	}
	query := map[string][]string{"repository_id": {strconv.FormatInt(repositoryID, 10)}}
	err := c.do(ctx, http.MethodGet, tenantPath(tenantID, "/runner-inventory"), query, nil, &body)
	var apiErr *Error
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
		switch apiErr.Code {
		case "installation_not_linked":
			return &InventoryOutcome{Kind: "not_linked"}, nil
		case "repository_not_authorized":
			return &InventoryOutcome{Kind: "not_authorized"}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	if err := required(body.RunnerInventory != nil && body.RunnerInventory.Profiles != nil); err != nil {
		return nil, err
	}
	return &InventoryOutcome{Kind: "available", Inventory: body.RunnerInventory}, nil
}
