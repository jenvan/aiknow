package repository

import "testing"

func TestSplitSQLDollarQuotedFunction(t *testing.T) {
	// PL/pgSQL 函数体内的 ';' 不应被切分
	sql := `
CREATE OR REPLACE FUNCTION f() RETURNS TRIGGER AS $$
BEGIN
    UPDATE t SET a = 1;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE INDEX i ON t (a);
`
	got := splitSQL(sql)
	if len(got) != 2 {
		t.Fatalf("期望 2 条语句，实际 %d 条：%#v", len(got), got)
	}
	if !contains(got[0], "RETURN NEW;") || !contains(got[0], "LANGUAGE plpgsql") {
		t.Errorf("函数体被切碎：%q", got[0])
	}
	if !contains(got[1], "CREATE INDEX") {
		t.Errorf("第二条语句错误：%q", got[1])
	}
}

func TestSplitSQLTaggedDollarQuote(t *testing.T) {
	sql := `CREATE FUNCTION g() RETURNS void AS $body$ SELECT 1; SELECT 2; $body$ LANGUAGE sql;`
	got := splitSQL(sql)
	if len(got) != 1 {
		t.Fatalf("期望 1 条语句，实际 %d 条：%#v", len(got), got)
	}
}

func TestSplitSQLPlaceholderNotDollarQuote(t *testing.T) {
	// $1 是参数占位符，不是美元引用起始
	sql := `INSERT INTO t VALUES ($1); SELECT 1;`
	got := splitSQL(sql)
	if len(got) != 2 {
		t.Fatalf("期望 2 条语句，实际 %d 条：%#v", len(got), got)
	}
}

func TestSplitSQLStringWithSemicolonAndEscapedQuote(t *testing.T) {
	sql := `SELECT 'a;b', 'it''s;fine'; SELECT 2;`
	got := splitSQL(sql)
	if len(got) != 2 {
		t.Fatalf("期望 2 条语句，实际 %d 条：%#v", len(got), got)
	}
	if !contains(got[0], "a;b") || !contains(got[0], "it''s;fine") {
		t.Errorf("字符串被切碎：%q", got[0])
	}
}

func TestSplitSQLCommentWithSemicolon(t *testing.T) {
	sql := `
-- 注释里有分号；不应切分 ; 这里
SELECT 1;
/* 块注释 ; 也不切分 */
SELECT 2;
`
	got := splitSQL(sql)
	if len(got) != 2 {
		t.Fatalf("期望 2 条语句，实际 %d 条：%#v", len(got), got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
