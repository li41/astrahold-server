package worldruntime

import (
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

func confirmLifecycleDelivery(r *Runtime, sessionID session.ID, message protocol.Message) {
	switch value := message.(type) {
	case protocol.EntitySpawn:
		r.replication.ConfirmSpawn(sessionID, value.EntityID)
		r.queueEntityVitalsForSession(sessionID, value.EntityID)
	case protocol.EntityDespawn:
		r.replication.ConfirmDespawn(sessionID, value.EntityID)
		r.confirmEntityDespawnVitals(sessionID, value.EntityID)
	}
}
