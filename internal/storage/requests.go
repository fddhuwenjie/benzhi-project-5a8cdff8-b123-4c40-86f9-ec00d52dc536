package storage

import "encoding/json"

func (s *Store) RequestJSON(id string, out any) bool {
	b, ok := s.GetRequest(id)
	if !ok {
		return false
	}
	return json.Unmarshal(b, out) == nil
}
