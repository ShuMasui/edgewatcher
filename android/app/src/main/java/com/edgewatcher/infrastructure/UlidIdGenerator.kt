package com.edgewatcher.infrastructure

import com.edgewatcher.domain.UlidGenerator
import com.edgewatcher.domain.port.IdGenerator
import java.time.Instant

class UlidIdGenerator : IdGenerator {
    override fun ulid(at: Instant): String = UlidGenerator.generate(at)
}
