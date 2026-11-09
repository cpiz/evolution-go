package instance_repository

import (
	"database/sql/driver"
	"encoding/json"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestUpdateConnectionSettingsWebhook(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		writeWebhook     bool
	}{
		{"omitted", `{}`, "https://current.example/webhook", false},
		{"null", `{"webhookUrl":null}`, "https://current.example/webhook", false},
		{"replace", `{"webhookUrl":"https://new.example/webhook"}`, "https://new.example/webhook", true},
		{"clear", `{"webhookUrl":""}`, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body struct {
				WebhookURL *string `json:"webhookUrl"`
			}
			if err := json.Unmarshal([]byte(tc.body), &body); err != nil {
				t.Fatal(err)
			}
			conn, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			db, err := gorm.Open(postgres.New(postgres.Config{Conn: conn}), &gorm.Config{SkipDefaultTransaction: true})
			if err != nil {
				t.Fatal(err)
			}
			inst := &instance_model.Instance{Id: "instance", Events: "GROUP,MESSAGE", Webhook: "https://stale.example/webhook"}
			sql := `UPDATE "instances" SET "events"=$1,"nats_enable"=$2,"rabbitmq_enable"=$3,"web_socket_enable"=$4`
			args := []driver.Value{inst.Events, "", "", ""}
			if tc.writeWebhook {
				sql += `,"webhook"=$5 WHERE "id" = $6 RETURNING *`
				args = append(args, tc.want)
			} else {
				sql += ` WHERE "id" = $5 RETURNING *`
			}
			mock.ExpectQuery(regexp.QuoteMeta(sql)).WithArgs(append(args, inst.Id)...).
				WillReturnRows(sqlmock.NewRows([]string{"id", "webhook"}).AddRow(inst.Id, tc.want))
			if err := (&instanceRepository{db: db}).UpdateConnectionSettings(inst, body.WebhookURL); err != nil {
				t.Fatal(err)
			}
			if inst.Webhook != tc.want {
				t.Fatalf("got webhook %q, want %q", inst.Webhook, tc.want)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
