package models

import (
	"time"

	"github.com/globalsign/mgo/bson"
)

const (
	ProfileUserdataTable MongoDbCollection = "profile_userdata"
)

type ProfileUserdataEntry struct {
	ID                   bson.ObjectId `bson:"_id,omitempty"`
	UserID               string
	Background           string
	BackgroundObjectName string
	Title                string
	Bio                  string
	Rep                  int
	LastRepped           time.Time
	ActiveBadgeIDs       []string
	BackgroundColor      string
	AccentColor          string
	TextColor            string
	BackgroundOpacity    string
	DetailOpacity        string
	BadgeOpacity         string
	EXPOpacity           string
	Timezone             string
	Birthday             string
	HideLastFm           bool
}
