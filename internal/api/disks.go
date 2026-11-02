package api

import (
	"context"
	"net/http"
	"net/url"
)

// Disk is a persistent disk as the customer API returns it. It never names
// a host.
type Disk struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	SizeGiB           int       `json:"size_gib"`
	Status            string    `json:"status"`
	AttachedSandboxID *string   `json:"attached_sandbox_id"`
	MountPath         string    `json:"mount_path"`
	CreatedAt         string    `json:"created_at"`
	DeletedAt         *string   `json:"deleted_at"`
	Usage             DiskUsage `json:"usage"`
}

// DiskUsage is the metered allocation, apart from compute minutes. It stays
// zero while disk charges are off.
type DiskUsage struct {
	GBHours     float64 `json:"gb_hours"`
	CreditsUsed float64 `json:"credits_used"`
}

// DiskQuota is what the workspace may keep, set by Runzivo staff.
type DiskQuota struct {
	QuotaGiB int `json:"quota_gib"`
	UsedGiB  int `json:"used_gib"`
	MaxDisks int `json:"max_disks"`
	Disks    int `json:"disks"`
}

// DiskListing is listCustomerDisks' result.
type DiskListing struct {
	Disks      []Disk    `json:"disks"`
	Quota      DiskQuota `json:"quota"`
	MinSizeGiB int       `json:"min_size_gib"`
	MaxSizeGiB int       `json:"max_size_gib"`
}

func (d *Disk) valid() bool {
	return nonEmpty(d.ID, d.Name, d.Status, d.CreatedAt) && d.SizeGiB > 0
}

func diskPath(tenantID, rest string) string { return tenantPath(tenantID, "/disks"+rest) }

// DiskList calls listCustomerDisks.
func (c *Client) DiskList(ctx context.Context, tenantID string) (*DiskListing, error) {
	var body DiskListing
	if err := c.do(ctx, http.MethodGet, diskPath(tenantID, ""), nil, nil, &body); err != nil {
		return nil, err
	}
	for i := range body.Disks {
		if err := required(body.Disks[i].valid()); err != nil {
			return nil, err
		}
	}
	if body.Disks == nil {
		body.Disks = []Disk{}
	}
	return &body, nil
}

// DiskGet calls getCustomerDisk.
func (c *Client) DiskGet(ctx context.Context, tenantID, diskID string) (*Disk, error) {
	return c.diskCall(ctx, http.MethodGet, diskPath(tenantID, "/"+url.PathEscape(diskID)), nil)
}

// CreateDiskInput is a create request. An empty ClientOperationID is left out.
type CreateDiskInput struct {
	Name              string `json:"name"`
	SizeGiB           int    `json:"size_gib"`
	ClientOperationID string `json:"client_operation_id,omitempty"`
}

// DiskCreate calls createCustomerDisk.
func (c *Client) DiskCreate(ctx context.Context, tenantID string, input CreateDiskInput) (*Disk, error) {
	return c.diskCall(ctx, http.MethodPost, diskPath(tenantID, ""), input)
}

// DiskDelete calls deleteCustomerDisk.
func (c *Client) DiskDelete(ctx context.Context, tenantID, diskID string) (*Disk, error) {
	return c.diskCall(ctx, http.MethodDelete, diskPath(tenantID, "/"+url.PathEscape(diskID)), nil)
}

// DiskAttach calls attachCustomerDisk.
func (c *Client) DiskAttach(ctx context.Context, tenantID, diskID, sandboxID string) (*Disk, error) {
	return c.diskCall(ctx, http.MethodPost, diskPath(tenantID, "/"+url.PathEscape(diskID)+"/attach"), map[string]string{"sandbox_id": sandboxID})
}

// DiskDetach calls detachCustomerDisk.
func (c *Client) DiskDetach(ctx context.Context, tenantID, diskID string) (*Disk, error) {
	return c.diskCall(ctx, http.MethodPost, diskPath(tenantID, "/"+url.PathEscape(diskID)+"/detach"), nil)
}

func (c *Client) diskCall(ctx context.Context, method, path string, body any) (*Disk, error) {
	var disk Disk
	if err := c.do(ctx, method, path, nil, body, &disk); err != nil {
		return nil, err
	}
	if err := required(disk.valid()); err != nil {
		return nil, err
	}
	return &disk, nil
}
