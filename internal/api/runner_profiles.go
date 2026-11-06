package api

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// CustomRunnerProfile is one custom runner profile (CustomRunnerProfile in
// the customer contract).
type CustomRunnerProfile struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Label         string `json:"label"`
	WorkflowLabel string `json:"workflow_label"`
	VCPU          int64  `json:"vcpu"`
	MemMiB        int64  `json:"mem_mib"`
	DiskGiB       int64  `json:"disk_gib"`
	CreatedAt     string `json:"created_at"`
}

// CustomRunnerProfileList is listCustomerCustomRunnerProfiles' answer.
type CustomRunnerProfileList struct {
	Profiles        []CustomRunnerProfile `json:"profiles"`
	IncludedDiskGiB int64                 `json:"included_disk_gib"`
}

// CreateCustomRunnerProfileInput is createCustomerCustomRunnerProfile's body.
type CreateCustomRunnerProfileInput struct {
	Name    string `json:"name"`
	Label   string `json:"label"`
	VCPU    int64  `json:"vcpu"`
	MemGiB  int64  `json:"mem_gib"`
	DiskGiB int64  `json:"disk_gib"`
}

// RunnerShape is a shape to check: vCPU, memory and temporary disk in GiB.
type RunnerShape struct {
	VCPU    int64
	MemGiB  int64
	DiskGiB int64
}

// CustomRunnerRegionAvailability is one region's answer.
type CustomRunnerRegionAvailability struct {
	Region        string `json:"region"`
	Status        string `json:"status"`
	Overcommitted bool   `json:"overcommitted"`
}

// CustomRunnerShapeAvailability is one shape's answer. Rate is
// equivalent_minutes_per_minute, empty when unpriced.
type CustomRunnerShapeAvailability struct {
	ProfileID     string                           `json:"profile_id,omitempty"`
	WorkflowLabel string                           `json:"workflow_label,omitempty"`
	Rate          string                           `json:"equivalent_minutes_per_minute,omitempty"`
	VCPU          int64                            `json:"vcpu"`
	MemGiB        int64                            `json:"mem_gib"`
	DiskGiB       int64                            `json:"disk_gib"`
	Status        string                           `json:"status"`
	Reason        string                           `json:"reason,omitempty"`
	Overcommitted bool                             `json:"overcommitted"`
	Regions       []CustomRunnerRegionAvailability `json:"regions"`
}

func customRunnerProfilesPath(tenantID, rest string) string {
	return tenantPath(tenantID, "/custom-runner-profiles"+rest)
}

func validProfile(profile CustomRunnerProfile) bool {
	return nonEmpty(profile.ID, profile.Name, profile.Label, profile.WorkflowLabel) && profile.VCPU > 0 && profile.MemMiB > 0 && profile.DiskGiB > 0
}

// CustomRunnerProfiles calls listCustomerCustomRunnerProfiles.
func (c *Client) CustomRunnerProfiles(ctx context.Context, tenantID string) (*CustomRunnerProfileList, error) {
	var body struct {
		Profiles        *[]CustomRunnerProfile `json:"profiles"`
		IncludedDiskGiB int64                  `json:"included_disk_gib"`
	}
	if err := c.do(ctx, http.MethodGet, customRunnerProfilesPath(tenantID, ""), nil, nil, &body); err != nil {
		return nil, err
	}
	if err := required(body.Profiles != nil); err != nil {
		return nil, err
	}
	for _, profile := range *body.Profiles {
		if err := required(validProfile(profile)); err != nil {
			return nil, err
		}
	}
	return &CustomRunnerProfileList{Profiles: *body.Profiles, IncludedDiskGiB: body.IncludedDiskGiB}, nil
}

// CreateCustomRunnerProfile calls createCustomerCustomRunnerProfile.
func (c *Client) CreateCustomRunnerProfile(ctx context.Context, tenantID string, input CreateCustomRunnerProfileInput) (*CustomRunnerProfile, error) {
	var profile CustomRunnerProfile
	if err := c.do(ctx, http.MethodPost, customRunnerProfilesPath(tenantID, ""), nil, input, &profile); err != nil {
		return nil, err
	}
	if err := required(validProfile(profile)); err != nil {
		return nil, err
	}
	return &profile, nil
}

// ArchiveCustomRunnerProfile calls archiveCustomerCustomRunnerProfile.
func (c *Client) ArchiveCustomRunnerProfile(ctx context.Context, tenantID, profileID string) error {
	return c.do(ctx, http.MethodDelete, customRunnerProfilesPath(tenantID, "/"+url.PathEscape(profileID)), nil, nil, nil)
}

// CustomRunnerProfileAvailability calls
// getCustomerCustomRunnerProfileAvailability: shape nil answers every live
// profile.
func (c *Client) CustomRunnerProfileAvailability(ctx context.Context, tenantID string, shape *RunnerShape) ([]CustomRunnerShapeAvailability, error) {
	var query url.Values
	if shape != nil {
		query = url.Values{
			"vcpu":     {strconv.FormatInt(shape.VCPU, 10)},
			"mem_gib":  {strconv.FormatInt(shape.MemGiB, 10)},
			"disk_gib": {strconv.FormatInt(shape.DiskGiB, 10)},
		}
	}
	var body struct {
		Shapes *[]CustomRunnerShapeAvailability `json:"shapes"`
	}
	if err := c.do(ctx, http.MethodGet, customRunnerProfilesPath(tenantID, "/availability"), query, nil, &body); err != nil {
		return nil, err
	}
	if err := required(body.Shapes != nil); err != nil {
		return nil, err
	}
	for _, shape := range *body.Shapes {
		if err := required(nonEmpty(shape.Status) && shape.Regions != nil); err != nil {
			return nil, err
		}
	}
	return *body.Shapes, nil
}
