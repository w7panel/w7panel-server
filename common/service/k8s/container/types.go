package container

type Progress struct {
	Content string `json:"content"`
}

type PushRequest struct {
	ImageName         string
	RegisterDomain    string
	RegistryUsername  string
	RegistryPassword  string
	RegistryPlainHTTP bool
	Progress          func(progress Progress) error
}
