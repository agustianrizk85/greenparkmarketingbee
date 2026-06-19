package handler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// MetaHandler proxies the Meta (Facebook) Graph API server-side so the access
// token stays out of the browser and CORS is avoided. It powers the Marketing
// Ads / WhatsApp / Instagram tabs.
type MetaHandler struct {
	token      string
	ver        string
	businessID string
	adAccount  string // pinned ad account id (without act_), optional
	http       *http.Client
}

func NewMetaHandler(token, ver, businessID, adAccount string) *MetaHandler {
	if ver == "" {
		ver = "v21.0"
	}
	return &MetaHandler{token: token, ver: ver, businessID: businessID, adAccount: adAccount, http: &http.Client{Timeout: 25 * time.Second}}
}

func (h *MetaHandler) configured() bool { return h.token != "" }

// graph performs an authenticated GET against the Graph API.
func (h *MetaHandler) graph(path string, params map[string]string) (map[string]any, error) {
	u, _ := url.Parse("https://graph.facebook.com/" + h.ver + path)
	q := u.Query()
	q.Set("access_token", h.token)
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	res, err := h.http.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func dataList(m map[string]any) []any {
	if m == nil {
		return nil
	}
	if d, ok := m["data"].([]any); ok {
		return d
	}
	return nil
}
func gstr(m map[string]any, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}

// gnum reads a numeric field that Graph may return as a string or a number.
func gnum(m map[string]any, k string) float64 {
	switch v := m[k].(type) {
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	case float64:
		return v
	}
	return 0
}

func spendOf(a map[string]any) float64 {
	switch v := a["amount_spent"].(type) {
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	case float64:
		return v
	}
	return 0
}

// pickAccount returns the account to detail: the pinned id when set (matched
// with or without the act_ prefix), otherwise the highest-spend account so an
// empty/test account is never the default. Returns nil when a pinned id is set
// but not present in the list (caller then reads it directly).
func pickAccount(list []any, pinned string) map[string]any {
	pin := pinned
	if pin != "" && (len(pin) < 4 || pin[:4] != "act_") {
		pin = "act_" + pin
	}
	var best map[string]any
	bestSpend := -1.0
	for _, it := range list {
		a, _ := it.(map[string]any)
		if a == nil {
			continue
		}
		if pin != "" && gstr(a, "id") == pin {
			return a
		}
		if s := spendOf(a); s > bestSpend {
			bestSpend, best = s, a
		}
	}
	if pinned != "" {
		return nil
	}
	return best
}

// Ads — the most complete pull in one call: every accessible ad account with
// its 30-day summary, plus a full per-campaign breakdown (spend / result /
// cost-per-result / CTR / CPC) across all accounts, results parsed from the
// Meta `actions` field.
func (h *MetaHandler) Ads(c *gin.Context) {
	if !h.configured() {
		c.JSON(http.StatusOK, gin.H{"configured": false})
		return
	}
	acc, err := h.graph("/me/adaccounts", map[string]string{"fields": "id,name,account_status,currency,amount_spent,balance", "limit": "100"})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"configured": true, "error": err.Error()})
		return
	}
	list := dataList(acc)

	allCampaigns := []gin.H{}
	var totSpend, totResults, totImpr, totClicks float64
	var totActive int
	insFields := "spend,impressions,reach,clicks,ctr,cpc,cpm,actions"

	for _, it := range list {
		a, _ := it.(map[string]any)
		if a == nil {
			continue
		}
		id := gstr(a, "id")
		// 30-day account summary attached to each account.
		if ins, e := h.graph("/"+id+"/insights", map[string]string{"date_preset": "last_30d", "fields": "spend,impressions,clicks,ctr"}); e == nil {
			if d := dataList(ins); len(d) > 0 {
				a["insights"] = d[0]
			}
		}
		// Campaign meta (status/objective) keyed by id.
		meta := map[string]map[string]any{}
		if cm, e := h.graph("/"+id+"/campaigns", map[string]string{"fields": "id,name,status,objective,daily_budget", "limit": "500"}); e == nil {
			for _, ci := range dataList(cm) {
				if cmap, ok := ci.(map[string]any); ok {
					meta[gstr(cmap, "id")] = cmap
				}
			}
		}
		// Per-campaign 30-day insights.
		ci, e := h.graph("/"+id+"/insights", map[string]string{
			"level": "campaign", "date_preset": "last_30d",
			"fields": "campaign_id,campaign_name," + insFields, "limit": "500",
		})
		if e != nil {
			continue
		}
		for _, row := range dataList(ci) {
			r, _ := row.(map[string]any)
			if r == nil {
				continue
			}
			cid := gstr(r, "campaign_id")
			label, results := resultFromActions(r)
			spend := gnum(r, "spend")
			impr := gnum(r, "impressions")
			clicks := gnum(r, "clicks")
			cpr := 0.0
			if results > 0 {
				cpr = spend / results
			}
			m := meta[cid]
			status := mstr(m, "status")
			allCampaigns = append(allCampaigns, gin.H{
				"id": cid, "name": gstr(r, "campaign_name"),
				"account":   gstr(a, "name"),
				"accountId": strings.TrimPrefix(id, "act_"),
				"status":    status,
				"objective": strings.TrimPrefix(mstr(m, "objective"), "OUTCOME_"),
				"spend":     spend, "impressions": impr, "clicks": clicks,
				"ctr": gnum(r, "ctr"), "cpc": gnum(r, "cpc"),
				"resultLabel": label, "results": results, "costPerResult": cpr,
			})
			totSpend += spend
			totResults += results
			totImpr += impr
			totClicks += clicks
			if status == "ACTIVE" {
				totActive++
			}
		}
	}

	// Sort campaigns by spend desc.
	sort.Slice(allCampaigns, func(i, j int) bool {
		return allCampaigns[i]["spend"].(float64) > allCampaigns[j]["spend"].(float64)
	})

	out := gin.H{"configured": true, "accounts": list, "campaigns": allCampaigns}
	chosen := pickAccount(list, h.adAccount)
	if chosen != nil {
		out["account"] = chosen
		if ins, ok := chosen["insights"].(map[string]any); ok {
			out["insights"] = ins
		}
	}
	cpr := 0.0
	if totResults > 0 {
		cpr = totSpend / totResults
	}
	ctr := 0.0
	if totImpr > 0 {
		ctr = totClicks / totImpr * 100
	}
	cpc := 0.0
	if totClicks > 0 {
		cpc = totSpend / totClicks
	}
	cpm := 0.0
	if totImpr > 0 {
		cpm = totSpend / totImpr * 1000
	}
	out["totals"] = gin.H{
		"spend": totSpend, "results": totResults, "costPerResult": cpr,
		"impressions": totImpr, "clicks": totClicks, "ctr": ctr, "cpc": cpc, "cpm": cpm,
		"campaigns": len(allCampaigns), "activeCampaigns": totActive,
		"accounts": len(list),
	}
	c.JSON(http.StatusOK, out)
}

// primaryAccountID resolves the ad account to break down: the pinned one, else
// the highest-spend account the token can see.
func (h *MetaHandler) primaryAccountID() string {
	if h.adAccount != "" {
		if strings.HasPrefix(h.adAccount, "act_") {
			return h.adAccount
		}
		return "act_" + h.adAccount
	}
	acc, _ := h.graph("/me/adaccounts", map[string]string{"fields": "id,amount_spent", "limit": "100"})
	if a := pickAccount(dataList(acc), ""); a != nil {
		return gstr(a, "id")
	}
	return ""
}

// insightRows fetches insights rows with the standard metric fields + actions.
func (h *MetaHandler) insightRows(act string, params map[string]string) []map[string]any {
	base := map[string]string{"date_preset": "last_30d", "limit": "500", "fields": "spend,impressions,clicks,ctr,actions"}
	for k, v := range params {
		base[k] = v
	}
	r, e := h.graph("/"+act+"/insights", base)
	if e != nil {
		return nil
	}
	out := []map[string]any{}
	for _, it := range dataList(r) {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// mapBreakdown turns insight rows into compact {label, spend, results, ...} and
// sorts by spend desc, keeping the top `limit` (0 = all).
func mapBreakdown(rows []map[string]any, label func(map[string]any) string, limit int) []gin.H {
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		_, res := resultFromActions(r)
		out = append(out, gin.H{
			"label": label(r), "spend": gnum(r, "spend"), "impressions": gnum(r, "impressions"),
			"clicks": gnum(r, "clicks"), "ctr": gnum(r, "ctr"), "results": res,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["spend"].(float64) > out[j]["spend"].(float64) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// AdsDetail — deep breakdowns for the primary account: daily trend, demographics
// (age/gender), placement, region, device, and top ads.
func (h *MetaHandler) AdsDetail(c *gin.Context) {
	if !h.configured() {
		c.JSON(http.StatusOK, gin.H{"configured": false})
		return
	}
	act := h.primaryAccountID()
	if act == "" {
		c.JSON(http.StatusOK, gin.H{"configured": true, "error": "tidak ada akun iklan"})
		return
	}

	// Daily trend (chronological).
	daily := []gin.H{}
	for _, r := range h.insightRows(act, map[string]string{"time_increment": "1"}) {
		_, res := resultFromActions(r)
		daily = append(daily, gin.H{"date": gstr(r, "date_start"), "spend": gnum(r, "spend"), "results": res, "clicks": gnum(r, "clicks"), "impressions": gnum(r, "impressions")})
	}

	demo := mapBreakdown(h.insightRows(act, map[string]string{"breakdowns": "age,gender"}),
		func(r map[string]any) string { return gstr(r, "age") + " · " + gstr(r, "gender") }, 12)
	placement := mapBreakdown(h.insightRows(act, map[string]string{"breakdowns": "publisher_platform,platform_position"}),
		func(r map[string]any) string { return gstr(r, "publisher_platform") + " · " + gstr(r, "platform_position") }, 12)
	region := mapBreakdown(h.insightRows(act, map[string]string{"breakdowns": "region"}),
		func(r map[string]any) string { return gstr(r, "region") }, 10)
	device := mapBreakdown(h.insightRows(act, map[string]string{"breakdowns": "impression_device"}),
		func(r map[string]any) string { return gstr(r, "impression_device") }, 10)
	topAds := mapBreakdown(h.insightRows(act, map[string]string{"level": "ad", "fields": "ad_name,spend,impressions,clicks,ctr,actions"}),
		func(r map[string]any) string { return gstr(r, "ad_name") }, 12)

	c.JSON(http.StatusOK, gin.H{
		"configured": true, "account": act,
		"daily": daily, "demographics": demo, "placements": placement,
		"regions": region, "devices": device, "topAds": topAds,
	})
}

// mstr reads a string field from a possibly-nil map.
func mstr(m map[string]any, k string) string {
	if m == nil {
		return ""
	}
	return gstr(m, k)
}

// resultFromActions picks the most meaningful conversion from Meta's `actions`
// array and returns a human label + count (messaging > lead > purchase > click).
func resultFromActions(r map[string]any) (string, float64) {
	actions, _ := r["actions"].([]any)
	vals := map[string]float64{}
	for _, it := range actions {
		a, _ := it.(map[string]any)
		if a == nil {
			continue
		}
		vals[gstr(a, "action_type")] = gnum(a, "value")
	}
	pri := []struct{ key, label string }{
		{"onsite_conversion.messaging_conversation_started_7d", "Percakapan WA"},
		{"onsite_conversion.total_messaging_connection", "Pesan"},
		{"onsite_conversion.lead_grouped", "Lead"},
		{"lead", "Lead"},
		{"purchase", "Pembelian"},
		{"link_click", "Klik Link"},
	}
	for _, p := range pri {
		if v, ok := vals[p.key]; ok && v > 0 {
			return p.label, v
		}
	}
	return "", 0
}

// WhatsApp — WhatsApp Business Accounts under the business + their phone numbers.
func (h *MetaHandler) WhatsApp(c *gin.Context) {
	if !h.configured() {
		c.JSON(http.StatusOK, gin.H{"configured": false})
		return
	}
	r, err := h.graph("/"+h.businessID+"/owned_whatsapp_business_accounts", map[string]string{"fields": "id,name,timezone_id,message_template_namespace"})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"configured": true, "error": err.Error()})
		return
	}
	wabas := []gin.H{}
	for _, it := range dataList(r) {
		w, _ := it.(map[string]any)
		id := gstr(w, "id")
		entry := gin.H{"id": id, "name": gstr(w, "name")}
		if ph, e := h.graph("/"+id+"/phone_numbers", map[string]string{"fields": "display_phone_number,verified_name,quality_rating,code_verification_status,platform_type"}); e == nil {
			entry["phones"] = dataList(ph)
		}
		if tpl, e := h.graph("/"+id+"/message_templates", map[string]string{"fields": "name,status,category", "limit": "100"}); e == nil {
			entry["templates"] = dataList(tpl)
		}
		wabas = append(wabas, entry)
	}
	c.JSON(http.StatusOK, gin.H{"configured": true, "wabas": wabas})
}

// Instagram — IG business accounts linked to the token's Pages (may be empty if
// no instagram_basic scope / no linked IG business account).
func (h *MetaHandler) Instagram(c *gin.Context) {
	if !h.configured() {
		c.JSON(http.StatusOK, gin.H{"configured": false})
		return
	}
	r, err := h.graph("/me/accounts", map[string]string{
		"fields": "id,name,fan_count,instagram_business_account{id,username,followers_count,media_count,profile_picture_url}",
		"limit":  "50",
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"configured": true, "error": err.Error()})
		return
	}
	pages := dataList(r)
	igs := []any{}
	for _, it := range pages {
		p, _ := it.(map[string]any)
		if ig, ok := p["instagram_business_account"].(map[string]any); ok {
			ig["page"] = gstr(p, "name")
			igs = append(igs, ig)
		}
	}
	c.JSON(http.StatusOK, gin.H{"configured": true, "pages": pages, "instagram": igs})
}
