package com.edgewatcher.domain

import com.edgewatcher.domain.model.PendingObservation

/**
 * オフラインバッファの上限を決める。
 *
 * 件数だけで制限すると高解像度設定でストレージを圧迫し、容量だけで制限すると
 * 低解像度設定で保持期間を超えた画像を大量に抱える。両方を置くことで、
 * どちらの設定でも破綻しない。
 *
 * バッファ上限を保持期間より長くしても意味がない。観測レコードの TTL は
 * capturedAt を基準に決まるため、長時間オフラインだった端末が古い画像を
 * 送っても、サーバ到着時点で期限切れになりうる。
 */
object BufferPolicy {

    /** 5分間隔で24時間相当。 */
    const val MAX_COUNT = 288

    /** 500MiB。 */
    const val MAX_BYTES = 524_288_000L

    /**
     * 削除すべき observationId を、古い順に返す。
     *
     * @param oldestFirst capturedAt の昇順であること。
     * @return 削除対象。何も落とす必要がなければ空。
     */
    fun evictions(oldestFirst: List<PendingObservation>): List<String> {
        var count = oldestFirst.size
        var bytes = oldestFirst.sumOf { it.totalBytes }

        val doomed = mutableListOf<String>()
        for (row in oldestFirst) {
            if (count <= MAX_COUNT && bytes <= MAX_BYTES) break
            doomed += row.observationId
            count -= 1
            bytes -= row.totalBytes
        }
        return doomed
    }
}
