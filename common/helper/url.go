package helper

import "net/url"

func RemoveQueryParam(rawURL string, keys ...string) string {
	uri, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	query := uri.Query()
	for _, key := range keys {
		query.Del(key)
	}
	uri.RawQuery = query.Encode()
	return uri.String()
}
