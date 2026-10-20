package workspace

// validatedSaveKeys is constructed from the same validated durable history before
// its ordinary epoch is published. It contains no receipts, actors, input digests
// or authority, and can only deny collision with process-local close history.
func (o *OpenOwner) validatedSaveKeys(ids []string) (map[string]struct{}, error) {
	keys := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if !recordID(id) {
			return nil, ErrOpen
		}
		if _, duplicate := keys[id]; duplicate {
			return nil, ErrOpen
		}
		keys[id] = struct{}{}
	}
	o.closeMu.Lock()
	defer o.closeMu.Unlock()
	for id := range keys {
		if o.closes[id] != nil {
			return nil, ErrSaveKeyReuse
		}
	}
	return keys, nil
}

// Only short metadata locks are acquired here. In particular, same-key close
// refusal cannot wait for this save's validation/admission/business IO. The
// ordinary cutoff is checked again immediately before persistence starts.
func (o *OpenOwner) reserveSaveKey(id string, investigationOnly bool) (bool, error) {
	o.closeMu.Lock()
	defer o.closeMu.Unlock()
	if o.closes[id] != nil {
		return false, ErrSaveKeyReuse
	}
	if err := o.queue.beforeMutation(); err != nil {
		return false, err
	}
	if _, known := o.saveKeys[id]; known || investigationOnly {
		return false, nil
	}
	o.saveKeys[id] = struct{}{}
	return true, nil
}
func (o *OpenOwner) forgetUnadmittedSaveKey(id string) {
	o.closeMu.Lock()
	delete(o.saveKeys, id)
	o.closeMu.Unlock()
}
