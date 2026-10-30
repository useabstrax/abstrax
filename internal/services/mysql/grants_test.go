package mysql

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestParseGrantFixtures(t *testing.T) {
	lines := []string{
		"GRANT USAGE ON *.* TO `app`@`localhost`",
		"GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, ALTER, INDEX, DROP, REFERENCES ON `shop`.* TO `app`@`localhost`",
		"GRANT ALL PRIVILEGES ON 'analytics'.* TO 'app'@'localhost'",
		"GRANT SELECT ON `shop`.`orders` TO `app`@`localhost`",
		"GRANT SELECT (email) ON `shop`.`users` TO `app`@`localhost`",
		"GRANT SELECT, INSERT ON `shop_%`.* TO `app`@`10.0.4.12`",
	}
	info := UserInfo{Name: "app", Host: "localhost", Grants: lines}
	attachParsedGrants(&info)

	if len(info.GlobalPrivileges) != 0 {
		t.Fatalf("global = %#v", info.GlobalPrivileges)
	}
	if len(info.Databases) != 3 {
		t.Fatalf("databases = %#v", info.Databases)
	}
	shop := info.Databases[0]
	if shop.Name != "shop" || shop.Preset == nil || *shop.Preset != PresetApp {
		t.Fatalf("shop = %#v", shop)
	}
	if info.Databases[1].Name != "analytics" || info.Databases[1].Preset == nil || *info.Databases[1].Preset != PresetAdmin {
		t.Fatalf("analytics = %#v", info.Databases[1])
	}
	if info.Databases[2].Name != "shop_%" || info.Databases[2].Preset != nil {
		t.Fatalf("wildcard = %#v", info.Databases[2])
	}
}

func TestPresetDetection(t *testing.T) {
	exact := matchPreset(splitPrivileges(PresetPrivileges[PresetReadonly]))
	if exact == nil || *exact != PresetReadonly {
		t.Fatalf("readonly = %v", exact)
	}
	subset := matchPreset([]string{"SELECT", "INSERT"})
	if subset != nil {
		t.Fatalf("subset matched %s", *subset)
	}
	superset := matchPreset([]string{"SELECT", "INSERT", "UPDATE", "DELETE", "CREATE", "ALTER", "INDEX", "DROP", "REFERENCES", "EXECUTE"})
	if superset != nil {
		t.Fatalf("superset matched %s", *superset)
	}
}

func TestUserListJSONWithGrantsKeepsEmptyLists(t *testing.T) {
	info := UserInfo{Name: "app", Host: "localhost", Grants: []string{"GRANT USAGE ON *.* TO `app`@`localhost`"}}
	attachParsedGrants(&info)
	b, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"databases":[]`) || !strings.Contains(string(b), `"global_privileges":[]`) {
		t.Fatalf("json = %s", b)
	}
}

func TestUserListJSONWithoutGrants(t *testing.T) {
	users := []UserInfo{{Name: "app", Host: "localhost"}}
	b, err := json.Marshal(users)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `[{"name":"app","host":"localhost"}]` {
		t.Fatalf("json = %s", b)
	}
}

func TestUserInfoFindsIPHost(t *testing.T) {
	svc := &Service{queryFn: func(_ context.Context, sql string) (string, error) {
		switch {
		case strings.Contains(sql, "SELECT Host FROM mysql.user"):
			return "Host\n10.0.4.12\n", nil
		case strings.Contains(sql, "SHOW GRANTS FOR 'reporting'@'10.0.4.12'"):
			return "Grants for reporting@10.0.4.12\nGRANT SELECT ON `reports`.* TO `reporting`@`10.0.4.12`\n", nil
		default:
			return "", errQuery(sql)
		}
	}}
	users, err := svc.UserInfo(context.Background(), "reporting", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].Host != "10.0.4.12" || len(users[0].Databases) != 1 || users[0].Databases[0].Name != "reports" {
		t.Fatalf("users = %#v", users)
	}

	_, err = svc.UserInfo(context.Background(), "reporting", "10.0.4.12")
	if err != nil {
		t.Fatal(err)
	}
}

type queryError string

func (e queryError) Error() string { return string(e) }

func errQuery(sql string) error { return queryError("unexpected query: " + sql) }
