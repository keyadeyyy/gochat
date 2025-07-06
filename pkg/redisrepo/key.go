package redisrepo

import (
	"fmt"
	"time"
)

func userSetKey() string {
	return "users"
}


func chatKey() string {
	return fmt.Sprintf("chat#%d", time.Now().UnixMilli())
}

func chatIndex() string {
	return "idx#chats"
}

func contactListZKey(username string) string {
	return "contacts:" + username
}
