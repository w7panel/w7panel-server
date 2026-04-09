package helper

func CloneMap[V any](src map[string]V) map[string]V {
	if src == nil {
		return nil
	}
	dst := make(map[string]V, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
