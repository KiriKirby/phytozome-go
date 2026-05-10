package lemnakeyword

import "github.com/KiriKirby/phytozome-go/internal/cachex"

var keywordCache = cachex.MustOpen("searchengine", "lemna", "keyword")

func readCachedJSON[T any](group string, key string) (T, bool) {
	var value T
	if !keywordCache.ReadJSON(group+":"+key, &value) {
		return value, false
	}
	return value, true
}

func writeCachedJSON(group string, key string, value any) {
	keywordCache.WriteJSON(group+":"+key, value)
}
