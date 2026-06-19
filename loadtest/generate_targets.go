//go:build ignore
// Generate Vegeta target files for load testing.
// Usage: go run generate_targets.go

package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
)

type Target struct {
	Method  string              `json:"method"`
	URL     string              `json:"url"`
	Headers map[string][]string `json:"header,omitempty"`
	Body    string              `json:"body,omitempty"`
}

const baseURL = "http://localhost:8080"

var products = []string{"G_ZJ", "G_MJ", "G_ZK", "G_N"}

func main() {
	os.MkdirAll("targets", 0755)

	genTrial("targets/trial.json", 10000)
	genLock("targets/lock.json", 5000)
	genIdempotentLock("targets/lock_idempotent.json", 1000, "IDEM-OUT-TRADE-001")
	genSettlement("targets/settlement.json", 5000)
	genSameTeamLock("targets/lock_same_team.json", 500, "TEAM-SHARED-001")

	fmt.Println("All target files generated in targets/")
}

func b64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

func genTrial(path string, count int) {
	f, _ := os.Create(path)
	defer f.Close()
	for range count {
		uid := fmt.Sprintf("U%06d", rand.Intn(1000)+1)
		gid := products[rand.Intn(len(products))]
		body := fmt.Sprintf(`{"user_id":"%s","goods_id":"%s","source":"APP","channel":"WECHAT"}`, uid, gid)
		t := Target{
			Method:  "POST",
			URL:     baseURL + "/api/v1/trial",
			Headers: map[string][]string{"Content-Type": {"application/json"}},
			Body:    b64(body),
		}
		json.NewEncoder(f).Encode(t)
	}
	fmt.Printf("  %s: %d requests\n", path, count)
}

func genLock(path string, count int) {
	f, _ := os.Create(path)
	defer f.Close()
	for i := range count {
		// 确保每个请求都是唯一用户（uid 从 100000 开始递增，避免重复）
		uid := fmt.Sprintf("U%06d", i+100000)
		outNo := fmt.Sprintf("LOADTEST-%s-%d", uid, i)
		body := fmt.Sprintf(`{"user_id":"%s","activity_id":200001,"goods_id":"G_ZJ","source":"APP","channel":"WECHAT","out_trade_no":"%s"}`,
			uid, outNo)
		t := Target{
			Method:  "POST",
			URL:     baseURL + "/api/v1/trade/lock",
			Headers: map[string][]string{"Content-Type": {"application/json"}},
			Body:    b64(body),
		}
		json.NewEncoder(f).Encode(t)
	}
	fmt.Printf("  %s: %d requests\n", path, count)
}

func genIdempotentLock(path string, count int, outTradeNo string) {
	f, _ := os.Create(path)
	defer f.Close()
	body := fmt.Sprintf(`{"user_id":"U_IDEM","activity_id":200001,"goods_id":"G_ZJ","source":"APP","channel":"WECHAT","out_trade_no":"%s"}`,
		outTradeNo)
	for range count {
		t := Target{
			Method:  "POST",
			URL:     baseURL + "/api/v1/trade/lock",
			Headers: map[string][]string{"Content-Type": {"application/json"}},
			Body:    b64(body),
		}
		json.NewEncoder(f).Encode(t)
	}
	fmt.Printf("  %s: %d requests (same out_trade_no=%s)\n", path, count, outTradeNo)
}

func genSettlement(path string, count int) {
	f, _ := os.Create(path)
	defer f.Close()
	for i := range count {
		uid := fmt.Sprintf("U%06d", rand.Intn(5000)+1000)
		outNo := fmt.Sprintf("LOADTEST-%s-%d", uid, i)
		body := fmt.Sprintf(`{"user_id":"%s","out_trade_no":"%s"}`, uid, outNo)
		t := Target{
			Method:  "POST",
			URL:     baseURL + "/api/v1/trade/settlement",
			Headers: map[string][]string{"Content-Type": {"application/json"}},
			Body:    b64(body),
		}
		json.NewEncoder(f).Encode(t)
	}
	fmt.Printf("  %s: %d requests\n", path, count)
}

func genSameTeamLock(path string, count int, teamID string) {
	f, _ := os.Create(path)
	defer f.Close()
	for i := range count {
		uid := fmt.Sprintf("U_TEAM%04d", i)
		outNo := fmt.Sprintf("TEAMTEST-%s-%d", uid, i)
		body := fmt.Sprintf(`{"user_id":"%s","activity_id":200001,"goods_id":"G_ZJ","source":"APP","channel":"WECHAT","out_trade_no":"%s","team_id":"%s"}`,
			uid, outNo, teamID)
		t := Target{
			Method:  "POST",
			URL:     baseURL + "/api/v1/trade/lock",
			Headers: map[string][]string{"Content-Type": {"application/json"}},
			Body:    b64(body),
		}
		json.NewEncoder(f).Encode(t)
	}
	fmt.Printf("  %s: %d requests (same team_id=%s)\n", path, count, teamID)
}
