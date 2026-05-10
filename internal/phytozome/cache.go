package phytozome

import "github.com/KiriKirby/phytozome-go/internal/cachex"

var phytozomeCache = cachex.MustOpen("phytozome")

func readCachedJSON[T any](group string, key string) (T, bool) {
	var value T
	if !phytozomeCache.ReadJSON(group+":"+key, &value) {
		return value, false
	}
	return value, true
}

func writeCachedJSON(group string, key string, value any) {
	phytozomeCache.WriteJSON(group+":"+key, value)
}

func readCachedText(group string, key string) (string, bool) {
	return phytozomeCache.ReadText(group + ":" + key)
}

func writeCachedText(group string, key string, value string) {
	phytozomeCache.WriteText(group+":"+key, value)
}
