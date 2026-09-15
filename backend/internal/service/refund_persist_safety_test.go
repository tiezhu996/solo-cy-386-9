package service

import "testing"

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

func TestGuardNoBusinessData(t *testing.T) {
	if err := guardNoBusinessData("marketpal_test", map[string]int64{}); err != nil {
		t.Fatalf("空库应放行，实际 %v", err)
	}
	empty := map[string]int64{"users": 0, "orders": 0, "products": 0, "refunds": 0, "refund_negotiations": 0}
	if err := guardNoBusinessData("marketpal_test", empty); err != nil {
		t.Fatalf("全部业务表为空应放行，实际 %v", err)
	}
	for table, n := range map[string]int64{"users": 1, "orders": 2, "products": 3, "refunds": 1, "refund_negotiations": 5} {
		data := map[string]int64{table: n}
		if err := guardNoBusinessData("marketpal_test", data); err == nil {
			t.Fatalf("%s 存在 %d 行业务数据时必须拒绝运行", table, n)
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
