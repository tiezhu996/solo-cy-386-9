package service

import (
	"testing"

	"github.com/marketpal/marketpal/internal/model"
)

func TestGuardDedicatedTestDB(t *testing.T) {
	token := pgConfirmToken
	cases := []struct {
		name      string
		dbName    string
		confirm   string
		allowName string
		wantErr   bool
	}{
		{"无确认令牌一律拒绝（即使库名像测试库）", "marketpal_test", "", "", true},
		{"令牌错误一律拒绝", "marketpal_test", "yes", "", true},
		{"确认且库名含 test 放行", "marketpal_test", token, "", false},
		{"确认且库名大写含 TEST 放行", "MARKETPAL_TEST", token, "", false},
		{"确认但指向生产库必须拒绝", "marketpal_db", token, "", true},
		{"确认且库名被精确放行", "ci_refund_db", token, "ci_refund_db", false},
		{"确认但放行名单指向别的库名仍拒绝", "marketpal_db", token, "ci_refund_db", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := guardDedicatedTestDB(tc.dbName, tc.confirm, tc.allowName)
			if (err != nil) != tc.wantErr {
				t.Fatalf("guardDedicatedTestDB(%q) err=%v, wantErr=%v", tc.dbName, err, tc.wantErr)
			}
		})
	}
}

func TestBusinessTableNamesFromModelRegistry(t *testing.T) {
	tables, err := businessTableNames()
	if err != nil {
		t.Fatalf("businessTableNames: %v", err)
	}
	// 必须覆盖全部持久化模型，包括此前漏检的 reviews/messages/audit_logs 等。
	for _, want := range []string{
		"users", "products", "favorites", "addresses", "cart_items",
		"orders", "refunds", "refund_negotiations", "messages", "reviews", "audit_logs",
	} {
		found := false
		for _, got := range tables {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("业务表检查清单缺少 %s，完整清单=%v", want, tables)
		}
	}
	if len(tables) != len(model.AllModels()) {
		t.Fatalf("检查表数 %d 与注册模型数 %d 不一致", len(tables), len(model.AllModels()))
	}
}

func TestGuardNoBusinessData(t *testing.T) {
	tables, err := businessTableNames()
	if err != nil {
		t.Fatalf("businessTableNames: %v", err)
	}
	// 空清单拒绝（无法确认检查范围）。
	if err := guardNoBusinessData("marketpal_test", nil, nil); err == nil {
		t.Fatal("空检查表必须拒绝运行")
	}
	// 全部表为空：放行。
	empty := make(map[string]int64, len(tables))
	for _, table := range tables {
		empty[table] = 0
	}
	if err := guardNoBusinessData("marketpal_test", tables, empty); err != nil {
		t.Fatalf("全部业务表为空应放行，实际 %v", err)
	}
	// 任意一张表（重点验证 reviews 等此前漏检表）有 1 行即拒绝。
	for _, table := range tables {
		data := map[string]int64{table: 1}
		if err := guardNoBusinessData("marketpal_test", tables, data); err == nil {
			t.Fatalf("%s 存在既有数据时必须拒绝运行", table)
		}
	}
}

func TestWithPostgresSearchPath(t *testing.T) {
	cases := []struct {
		name string
		dsn  string
		want string
	}{
		{
			name: "key-value DSN",
			dsn:  "host=localhost port=5432 user=u dbname=marketpal_test sslmode=disable",
			want: "host=localhost port=5432 user=u dbname=marketpal_test sslmode=disable search_path=rt_refund_1",
		},
		{
			name: "url DSN without query",
			dsn:  "postgres://u:p@localhost:5432/marketpal_test",
			want: "postgres://u:p@localhost:5432/marketpal_test?search_path=rt_refund_1",
		},
		{
			name: "url DSN with query",
			dsn:  "postgres://u:p@localhost:5432/marketpal_test?sslmode=disable",
			want: "postgres://u:p@localhost:5432/marketpal_test?sslmode=disable&search_path=rt_refund_1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := withPostgresSearchPath(tc.dsn, "rt_refund_1"); got != tc.want {
				t.Fatalf("withPostgresSearchPath() = %q, want %q", got, tc.want)
			}
		})
	}
}
