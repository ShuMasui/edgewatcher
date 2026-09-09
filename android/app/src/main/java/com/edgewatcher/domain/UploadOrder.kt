package com.edgewatcher.domain

import com.edgewatcher.domain.model.PendingObservation

/**
 * 送信の順序を決める。
 *
 * **最新の1枚を先に送り、そのあと残りを古い順に送る。**
 *
 * 長時間オフラインから復帰したとき、単純な FIFO で古い順に送ると、Web の
 * ダッシュボードには何時間も前の画像が出続ける。オーナーが最初に知りたいのは
 * 「今どうなっているか」であり、それが即座に反映されるようにする。
 *
 * 遅れて届いた古い画像が Device.latestThumbnailKey を巻き戻さないよう、
 * サーバ側は条件付き更新を行っている（engineering/dynamodb.md §5.1）。
 * したがって端末はこの順序を素直に守ってよい。
 */
object UploadOrder {

    /**
     * @param oldestFirst 通常は capturedAt の昇順。順不同で渡されても正しく並べる。
     */
    fun order(oldestFirst: List<PendingObservation>): List<PendingObservation> {
        if (oldestFirst.size <= 1) return oldestFirst

        val sorted = oldestFirst.sortedBy { it.capturedAt }
        val newest = sorted.last()
        return listOf(newest) + sorted.dropLast(1)
    }
}
