package main

func (a *durableSessionLoginAuthenticator) RecoveryPasswordPolicy() (argon2idPasswordHash, bool) {
	if a == nil || !a.RecoveryEnabled() || len(a.dummy.digest) != sessionPasswordArgon2DigestBytes {
		return argon2idPasswordHash{}, false
	}
	return cloneArgon2idPasswordHash(a.dummy), true
}
