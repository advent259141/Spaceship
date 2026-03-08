package policy

type Engine struct{}

func (Engine) AllowPath(path string) bool {
	_ = path
	return true
}
