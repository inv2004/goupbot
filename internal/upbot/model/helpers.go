package model

func (k JobInfoKey) Key() string {
	return (k.User + ";" + k.GUID)
}
