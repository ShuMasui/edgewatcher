package com.edgewatcher.infrastructure

import com.edgewatcher.domain.port.Clock
import java.time.Instant

class SystemClock : Clock {
    override fun now(): Instant = Instant.now()
}
