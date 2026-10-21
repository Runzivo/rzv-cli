package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
	"unicode/utf8"

	"github.com/Runzivo/rzv-cli/internal/api"
)

const aiUsage = `Usage:
  rzv ai providers list [--workspace <id>] [--json]
  rzv ai providers add --kind <identifier> [--label <label>] [--base-url <url>] [--key-file <path>] [--workspace <id>] [--json]
  rzv ai providers update <provider-id> [--label <label>] [--base-url <url>] [--key-file <path>] [--workspace <id>] [--json]
  rzv ai providers remove <provider-id> [--workspace <id>] [--json]
  rzv ai providers test <provider-id> [--workspace <id>] [--json]
  rzv ai providers models <provider-id> [--refresh] [--workspace <id>] [--json]
  rzv ai settings show [--workspace <id>] [--json]
  rzv ai settings update --file <path> [--workspace <id>] [--json]
  rzv ai usage summary [--from <RFC3339|YYYY-MM-DD>] [--to <RFC3339|YYYY-MM-DD>] [--group-by <model|agent|member|provider|day>] [--workspace <id>] [--json]
  rzv ai usage calls [--from <RFC3339|YYYY-MM-DD>] [--to <RFC3339|YYYY-MM-DD>] [--cursor <cursor>] [--limit <1-200>] [--workspace <id>] [--json]
  rzv ai usage limit [--workspace <id>] [--json]

Provider identifiers are passed to the service. Credential files are read only for
the request and are never stored or printed. Settings update uses a JSON partial
PUT: omitted fields keep their current value; use null to clear a monthly limit.`

func init() {
	register("ai", "Manage workspace AI providers, settings, and usage", runAI)
	commandUsage["ai"] = aiUsage
}

func runAI(a *app, cmd *command) error {
	if len(cmd.args) < 2 {
		return usageError("%s", aiUsage)
	}
	switch cmd.args[0] {
	case "providers":
		return aiProviders(a, cmd)
	case "settings":
		return aiSettings(a, cmd)
	case "usage":
		return aiUsageCommand(a, cmd)
	default:
		return usageError("%s", aiUsage)
	}
}
func aiID(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", usageError("<provider-id> must not be blank.")
	}
	return raw, nil
}
func aiReadFile(path string, maximum int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, int64(maximum)+1))
	if err != nil || len(data) > maximum {
		return nil, fmt.Errorf("too large")
	}
	return data, nil
}
func aiReadKey(cmd *command) (*string, error) {
	p, err := cmd.option("key-file")
	if err != nil || p == "" {
		return nil, err
	}
	data, err := aiReadFile(p, 514)
	if err != nil {
		return nil, usageError("--key-file could not be read.")
	}
	key := strings.TrimSpace(string(data))
	if len(key) < 8 || len(key) > 512 || strings.ContainsAny(key, " \t\r\n") {
		return nil, usageError("--key-file must contain one credential between 8 and 512 bytes.")
	}
	return &key, nil
}
func aiProviderInput(cmd *command, create bool) (api.AIProviderInput, error) {
	label, err := cmd.option("label")
	if err != nil {
		return api.AIProviderInput{}, err
	}
	base, err := cmd.option("base-url")
	if err != nil {
		return api.AIProviderInput{}, err
	}
	if label != "" && utf8.RuneCountInString(label) > 80 {
		return api.AIProviderInput{}, usageError("--label must be at most 80 characters.")
	}
	if len(base) > 500 || strings.ContainsAny(base, " \t\r\n") {
		return api.AIProviderInput{}, usageError("--base-url must be at most 500 bytes and contain no whitespace.")
	}
	key, err := aiReadKey(cmd)
	if err != nil {
		return api.AIProviderInput{}, err
	}
	in := api.AIProviderInput{APIKey: key}
	if label != "" {
		in.Label = &label
	}
	if base != "" {
		in.BaseURL = &base
	}
	if create {
		kind, err := cmd.required("kind")
		if err != nil {
			return in, err
		}
		in.Kind = kind
	}
	return in, nil
}
func aiProviders(a *app, cmd *command) error {
	switch cmd.args[1] {
	case "list":
		if err := cmd.allow("workspace", "account"); err != nil {
			return err
		}
		if err := cmd.expectArgs(2, "rzv ai providers list"); err != nil {
			return err
		}
		id, c, e := a.workspaceClient(cmd)
		if e != nil {
			return e
		}
		v, e := c.AIProviders(a.ctx, id)
		if e != nil {
			return apiError(e, "The AI provider was not found in this workspace.")
		}
		a.output(cmd, map[string]any{"workspace_id": id, "providers": v}, renderAIProviders(id, v))
		return nil
	case "add":
		if err := a.requireHuman(); err != nil {
			return err
		}
		if err := cmd.allow("workspace", "account", "kind", "label", "base-url", "key-file"); err != nil {
			return err
		}
		if err := cmd.expectArgs(2, "rzv ai providers add --kind <identifier>"); err != nil {
			return err
		}
		in, e := aiProviderInput(cmd, true)
		if e != nil {
			return e
		}
		id, c, e := a.workspaceClient(cmd)
		if e != nil {
			return e
		}
		v, e := c.CreateAIProvider(a.ctx, id, in)
		if e != nil {
			return apiError(e, "The AI provider was not found in this workspace.")
		}
		a.output(cmd, map[string]any{"workspace_id": id, "provider": v}, renderAIProvider(id, v))
		return nil
	case "update":
		if err := a.requireHuman(); err != nil {
			return err
		}
		if err := cmd.allow("workspace", "account", "label", "base-url", "key-file"); err != nil {
			return err
		}
		if err := cmd.expectArgs(3, "rzv ai providers update <provider-id>"); err != nil {
			return err
		}
		p, e := aiID(cmd.args[2])
		if e != nil {
			return e
		}
		in, e := aiProviderInput(cmd, false)
		if e != nil {
			return e
		}
		if in.Label == nil && in.BaseURL == nil && in.APIKey == nil {
			return usageError("Provide --label, --base-url, or --key-file.")
		}
		id, c, e := a.workspaceClient(cmd)
		if e != nil {
			return e
		}
		v, e := c.UpdateAIProvider(a.ctx, id, p, in)
		if e != nil {
			return apiError(e, "The AI provider was not found in this workspace.")
		}
		a.output(cmd, map[string]any{"workspace_id": id, "provider": v}, renderAIProvider(id, v))
		return nil
	case "remove":
		if err := a.requireHuman(); err != nil {
			return err
		}
		if err := cmd.allow("workspace", "account"); err != nil {
			return err
		}
		if err := cmd.expectArgs(3, "rzv ai providers remove <provider-id>"); err != nil {
			return err
		}
		p, e := aiID(cmd.args[2])
		if e != nil {
			return e
		}
		id, c, e := a.workspaceClient(cmd)
		if e != nil {
			return e
		}
		if e = c.DeleteAIProvider(a.ctx, id, p); e != nil {
			return apiError(e, "The AI provider was not found in this workspace.")
		}
		a.output(cmd, map[string]any{"workspace_id": id, "provider_id": p, "removed": true}, "Removed AI provider "+display(p)+".")
		return nil
	case "test":
		if err := a.requireHuman(); err != nil {
			return err
		}
		if err := cmd.allow("workspace", "account"); err != nil {
			return err
		}
		if err := cmd.expectArgs(3, "rzv ai providers test <provider-id>"); err != nil {
			return err
		}
		p, e := aiID(cmd.args[2])
		if e != nil {
			return e
		}
		id, c, e := a.workspaceClient(cmd)
		if e != nil {
			return e
		}
		v, e := c.TestAIProvider(a.ctx, id, p)
		if e != nil {
			return apiError(e, "The AI provider was not found in this workspace.")
		}
		a.output(cmd, map[string]any{"workspace_id": id, "test": v}, fmt.Sprintf("Provider %s: %s (%d ms), error code %s", display(p), display(v.Status), v.LatencyMS, displayOrDash(v.ErrorCode)))
		return nil
	case "models":
		if err := cmd.allow("workspace", "account", "refresh"); err != nil {
			return err
		}
		if err := cmd.expectArgs(3, "rzv ai providers models <provider-id> [--refresh]"); err != nil {
			return err
		}
		p, e := aiID(cmd.args[2])
		if e != nil {
			return e
		}
		id, c, e := a.workspaceClient(cmd)
		if e != nil {
			return e
		}
		v, at, e := c.AIModels(a.ctx, id, p, cmd.flags["refresh"])
		if e != nil {
			return apiError(e, "The AI provider was not found in this workspace.")
		}
		a.output(cmd, map[string]any{"workspace_id": id, "provider_id": p, "models": v, "fetched_at": at}, renderAIModels(p, v, at))
		return nil
	default:
		return usageError("%s", aiUsage)
	}
}
func aiSettings(a *app, cmd *command) error {
	switch cmd.args[1] {
	case "show":
		if err := cmd.allow("workspace", "account"); err != nil {
			return err
		}
		if err := cmd.expectArgs(2, "rzv ai settings show"); err != nil {
			return err
		}
		id, c, e := a.workspaceClient(cmd)
		if e != nil {
			return e
		}
		v, e := c.AISettings(a.ctx, id)
		if e != nil {
			return apiError(e, "")
		}
		a.output(cmd, map[string]any{"workspace_id": id, "settings": v}, renderAISettings(id, v))
		return nil
	case "update":
		if err := a.requireHuman(); err != nil {
			return err
		}
		if err := cmd.allow("workspace", "account", "file"); err != nil {
			return err
		}
		if err := cmd.expectArgs(2, "rzv ai settings update --file <path>"); err != nil {
			return err
		}
		raw, e := aiSettingsFile(cmd)
		if e != nil {
			return e
		}
		id, c, e := a.workspaceClient(cmd)
		if e != nil {
			return e
		}
		v, e := c.UpdateAISettings(a.ctx, id, raw)
		if e != nil {
			return apiError(e, "")
		}
		a.output(cmd, map[string]any{"workspace_id": id, "settings": v}, renderAISettings(id, v))
		return nil
	default:
		return usageError("%s", aiUsage)
	}
}
func aiSettingsFile(cmd *command) (json.RawMessage, error) {
	p, e := cmd.required("file")
	if e != nil {
		return nil, e
	}
	b, e := aiReadFile(p, 16<<10)
	if e != nil {
		return nil, usageError("--file could not be read.")
	}
	if len(b) == 0 || len(b) > 16<<10 {
		return nil, usageError("--file must contain a JSON settings request no larger than 16 KiB.")
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	var fields map[string]json.RawMessage
	if dec.Decode(&fields) != nil || fields == nil || dec.Decode(&struct{}{}) != io.EOF {
		return nil, usageError("--file must contain one valid AI settings JSON object with supported fields.")
	}
	allowed := map[string]bool{"mode": true, "agents": true, "autonomy": true, "monthly_token_limit": true, "monthly_cost_limit_cents": true, "allowed_roles": true}
	for key := range fields {
		if !allowed[key] {
			return nil, usageError("--file must contain one valid AI settings JSON object with supported fields.")
		}
	}
	var v struct {
		Mode                  *string                        `json:"mode"`
		Agents                map[string]api.AIAgentSettings `json:"agents"`
		Autonomy              *string                        `json:"autonomy"`
		MonthlyTokenLimit     *json.RawMessage               `json:"monthly_token_limit"`
		MonthlyCostLimitCents *json.RawMessage               `json:"monthly_cost_limit_cents"`
		AllowedRoles          []string                       `json:"allowed_roles"`
	}
	if json.Unmarshal(b, &v) != nil {
		return nil, usageError("--file must contain one valid AI settings JSON object with supported fields.")
	}
	if v.Mode != nil && !map[string]bool{"off": true, "byo": true, "runzivo": true}[*v.Mode] {
		return nil, usageError("mode must be off, byo, or runzivo.")
	}
	if v.Autonomy != nil && !map[string]bool{"suggest": true, "ask": true, "act": true}[*v.Autonomy] {
		return nil, usageError("autonomy must be suggest, ask, or act.")
	}
	if _, ok := fields["allowed_roles"]; ok {
		if len(v.AllowedRoles) == 0 || len(v.AllowedRoles) > 3 {
			return nil, usageError("allowed_roles must name one to three roles.")
		}
		seen := map[string]bool{}
		for _, role := range v.AllowedRoles {
			if !map[string]bool{"owner": true, "admin": true, "member": true}[role] || seen[role] {
				return nil, usageError("allowed_roles contains an invalid or repeated role.")
			}
			seen[role] = true
		}
	}
	if agentsRaw, ok := fields["agents"]; ok {
		if v.Agents == nil {
			return nil, usageError("agents must be an object.")
		}
		var rawAgents map[string]json.RawMessage
		if json.Unmarshal(agentsRaw, &rawAgents) != nil {
			return nil, usageError("agents must be an object.")
		}
		for agent, choice := range v.Agents {
			decoder := json.NewDecoder(bytes.NewReader(rawAgents[agent]))
			decoder.DisallowUnknownFields()
			var check api.AIAgentSettings
			if decoder.Decode(&check) != nil || decoder.Decode(&struct{}{}) != io.EOF {
				return nil, usageError("agents contains an invalid agent configuration.")
			}
			var choiceFields map[string]json.RawMessage
			if json.Unmarshal(rawAgents[agent], &choiceFields) != nil || choiceFields == nil || choiceFields["enabled"] == nil || choiceFields["provider_id"] == nil || choiceFields["model"] == nil {
				return nil, usageError("agents contains an invalid agent configuration.")
			}
			if !map[string]bool{"assistant": true, "pipeline_builder": true, "fix_agent": true}[agent] || (choice.Enabled && (choice.ProviderID == nil || choice.Model == nil)) || (choice.Model != nil && len(*choice.Model) > 200) {
				return nil, usageError("agents contains an invalid agent configuration.")
			}
		}
	}
	for _, n := range []struct {
		raw  *json.RawMessage
		max  int64
		name string
	}{{v.MonthlyTokenLimit, 1000000000000, "monthly_token_limit"}, {v.MonthlyCostLimitCents, 100000000, "monthly_cost_limit_cents"}} {
		if n.raw != nil && string(*n.raw) != "null" {
			var x int64
			if json.Unmarshal(*n.raw, &x) != nil || x < 1 || x > n.max {
				return nil, usageError("%s must be null or within its allowed range.", n.name)
			}
		}
	}
	return json.RawMessage(b), nil
}
func aiUsageCommand(a *app, cmd *command) error {
	if len(cmd.args) != 2 {
		return usageError("%s", aiUsage)
	}
	switch cmd.args[1] {
	case "summary":
		if e := cmd.allow("workspace", "account", "from", "to", "group-by"); e != nil {
			return e
		}
		q, e := aiQuery(cmd, false)
		if e != nil {
			return e
		}
		id, c, e := a.workspaceClient(cmd)
		if e != nil {
			return e
		}
		v, e := c.AIUsageSummary(a.ctx, id, q)
		if e != nil {
			return apiError(e, "")
		}
		a.output(cmd, map[string]any{"workspace_id": id, "usage": v}, renderAIUsageSummary(id, v))
		return nil
	case "calls":
		if e := cmd.allow("workspace", "account", "from", "to", "cursor", "limit"); e != nil {
			return e
		}
		q, e := aiQuery(cmd, true)
		if e != nil {
			return e
		}
		id, c, e := a.workspaceClient(cmd)
		if e != nil {
			return e
		}
		v, next, e := c.AICalls(a.ctx, id, q)
		if e != nil {
			return apiError(e, "")
		}
		a.output(cmd, map[string]any{"workspace_id": id, "calls": v, "next_cursor": next}, renderAICalls(id, v, next))
		return nil
	case "limit":
		if e := cmd.allow("workspace", "account"); e != nil {
			return e
		}
		id, c, e := a.workspaceClient(cmd)
		if e != nil {
			return e
		}
		v, e := c.AILimit(a.ctx, id)
		if e != nil {
			return apiError(e, "")
		}
		a.output(cmd, map[string]any{"workspace_id": id, "limit": v}, renderAILimit(id, v))
		return nil
	default:
		return usageError("%s", aiUsage)
	}
}
func aiQuery(cmd *command, calls bool) (url.Values, error) {
	q := url.Values{}
	for _, n := range []string{"from", "to", "cursor"} {
		v, e := cmd.option(n)
		if e != nil {
			return nil, e
		}
		if (n == "from" || n == "to") && v != "" && !aiDate(v) {
			return nil, usageError("--%s must be RFC 3339 or YYYY-MM-DD.", n)
		}
		if n == "cursor" && len(v) > 200 {
			return nil, usageError("--cursor must be at most 200 bytes.")
		}
		if v != "" {
			q.Set(n, v)
		}
	}
	g, e := cmd.option("group-by")
	if e != nil {
		return nil, e
	}
	if g != "" {
		if !map[string]bool{"model": true, "agent": true, "member": true, "provider": true, "day": true}[g] {
			return nil, usageError("--group-by must be model, agent, member, provider, or day.")
		}
		q.Set("group_by", g)
	}
	if calls {
		v, e := cmd.option("limit")
		if e != nil {
			return nil, e
		}
		if v != "" {
			n, e := strconv.Atoi(v)
			if e != nil || n < 1 || n > 200 {
				return nil, usageError("--limit must be an integer from 1 to 200.")
			}
			q.Set("limit", v)
		}
	}
	return q, nil
}
func aiDate(value string) bool {
	if _, err := time.Parse(time.RFC3339, value); err == nil {
		return true
	}
	_, err := time.Parse("2006-01-02", value)
	return err == nil
}
func renderAIProviders(id string, v []api.AIProvider) string {
	if len(v) == 0 {
		return "No AI providers for workspace " + display(id) + "."
	}
	var table strings.Builder
	w := tabwriter.NewWriter(&table, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tLABEL\tKIND\tBASE URL\tKEY HINT\tKEY\tSTATUS\tTESTED\tLAST ERROR\tCREATED\tUPDATED")
	for _, p := range v {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%t\t%s\t%s\t%s\t%s\t%s\n", display(p.ProviderID), display(p.Label), display(p.Kind), displayOrDash(p.BaseURL), displayOrDash(p.KeyHint), p.HasKey, display(p.Status), displayOrDash(p.LastTestedAt), displayOrDash(p.LastError), display(p.CreatedAt), display(p.UpdatedAt))
	}
	_ = w.Flush()
	return "AI providers for workspace " + display(id) + ":\n" + strings.TrimSuffix(table.String(), "\n")
}
func renderAIProvider(id string, p *api.AIProvider) string {
	return fmt.Sprintf("AI provider %s [%s] in workspace %s.", display(p.Label), display(p.ProviderID), display(id))
}
func renderAIModels(p string, v []api.AIModel, at *string) string {
	var table strings.Builder
	w := tabwriter.NewWriter(&table, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tNAME\tCONTEXT\tRECOMMENDED\tCREATED")
	for _, m := range v {
		context := "-"
		if m.ContextWindow != nil {
			context = strconv.FormatInt(*m.ContextWindow, 10)
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%t\t%s\n", display(m.ID), display(m.DisplayName), context, m.Recommended, displayOrDash(m.CreatedAt))
	}
	_ = w.Flush()
	fetched := "-"
	if at != nil {
		fetched = display(*at)
	}
	return "Models for provider " + display(p) + " (fetched " + fetched + "):\n" + strings.TrimSuffix(table.String(), "\n")
}
func renderAISettings(id string, v *api.AISettings) string {
	lines := []string{fmt.Sprintf("AI settings for workspace %s:", display(id)), "Mode: " + display(v.Mode), "Autonomy: " + display(v.Autonomy), "Monthly token limit: " + displayOrDash(intString(v.MonthlyTokenLimit)), "Monthly cost limit (cents): " + displayOrDash(intString(v.MonthlyCostLimitCents)), "Currency: " + display(v.Currency), "Allowed roles: " + aiDisplayList(v.AllowedRoles), fmt.Sprintf("BYO available: %t", v.BYOAvailable), fmt.Sprintf("Service AI available: %t", v.RunzivoAvailable), fmt.Sprintf("Can manage: %t", v.CanManage), "Updated: " + displayOrDash(v.UpdatedAt), "Allowed networks: " + aiDisplayList(v.AllowedNetworks), "Agents:"}
	for agent, choice := range v.Agents {
		lines = append(lines, fmt.Sprintf("- %s: enabled=%t provider=%s model=%s", display(agent), choice.Enabled, displayOrDash(choice.ProviderID), displayOrDash(choice.Model)))
	}
	return strings.Join(lines, "\n")
}
func intString(v *int64) *string {
	if v == nil {
		return nil
	}
	s := strconv.FormatInt(*v, 10)
	return &s
}

func aiDisplayList(values []string) string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = display(value)
	}
	return strings.Join(out, ", ")
}
func aggregate(v api.AIUsageAggregate) string {
	return fmt.Sprintf("requests %d, input %d, output %d, cached %d, cost %s, unpriced %d, errors %d, limited %d, p50 %s ms, p95 %s ms", v.Requests, v.InputTokens, v.OutputTokens, v.CachedTokens, displayOrDash(intString(v.CostEstimateMicros)), v.UnpricedRequests, v.Errors, v.Limited, displayOrDash(intString(v.P50LatencyMS)), displayOrDash(intString(v.P95LatencyMS)))
}
func renderAIUsageSummary(id string, v *api.AIUsageSummary) string {
	lines := []string{fmt.Sprintf("AI usage for workspace %s:", display(id)), "Period: " + display(v.From) + " to " + display(v.To), "Group: " + displayOrDash(v.GroupBy), "Totals: " + aggregate(*v.Totals)}
	if len(v.Series) > 0 {
		lines = append(lines, "Series:")
		for _, item := range v.Series {
			lines = append(lines, "- "+display(item.Key)+": "+aggregate(item))
		}
	}
	return strings.Join(lines, "\n")
}
func renderAICalls(id string, v []api.AICall, next *string) string {
	var table strings.Builder
	w := tabwriter.NewWriter(&table, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tCREATED\tAGENT\tMODE\tPROVIDER\tMODEL\tINPUT\tOUTPUT\tCACHED\tCOST\tLATENCY\tOUTCOME\tERROR\tACTOR")
	for _, call := range v {
		_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%d\t%d\t%s\t%s\t%d\t%s\t%s\t%s\n", call.ID, display(call.CreatedAt), display(call.Agent), display(call.Mode), display(call.ProviderKind), display(call.Model), call.InputTokens, call.OutputTokens, displayOrDash(intString(call.CachedTokens)), displayOrDash(intString(call.CostEstimateMicros)), call.LatencyMS, display(call.Outcome), displayOrDash(call.ErrorCode), displayOrDash(call.Actor))
	}
	_ = w.Flush()
	return "AI calls for workspace " + display(id) + " (next cursor " + displayOrDash(next) + "):\n" + strings.TrimSuffix(table.String(), "\n")
}
func renderAILimit(id string, v *api.AILimit) string {
	return fmt.Sprintf("AI limit for workspace %s:\nPeriod: %s to %s\nUsed tokens: %d\nUsed cost (micros): %s\nToken limit: %s\nCost limit (cents): %s\nLimited: %t\nLimit kind: %s", display(id), display(v.PeriodStart), display(v.PeriodEnd), v.UsedTokens, displayOrDash(intString(v.UsedCostMicros)), displayOrDash(intString(v.TokenLimit)), displayOrDash(intString(v.CostLimitCents)), v.Limited, displayOrDash(v.LimitKind))
}
