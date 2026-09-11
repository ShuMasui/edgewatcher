package com.edgewatcher.domain

/**
 * 「どう符号化するか」の判断を持つ。画素の操作は JpegEncoder が行う。
 *
 * 長辺・品質・落とす段階は方針であって画像処理ではないので domain に置く。
 */
object ImagePolicy {

    data class EncodeStep(val longestSide: Int, val quality: Int)

    /** 本画像 + サムネイルの合計の上限。超過はサーバが 400 を返し、リトライ不能。 */
    const val MAX_TOTAL_BYTES = 4_500_000L

    /**
     * サムネイル。長辺480px。
     *
     * Web のダッシュボードは端末カードの画像に latestThumbnailUrl をそのまま
     * 使っており、カード幅は 280〜360px ある。160px では明確にぼやける。
     * 480px なら履歴のコマ列（表示は 80〜120px）にも耐える。
     */
    val THUMBNAIL = EncodeStep(longestSide = 480, quality = 75)

    /**
     * 本画像を符号化する段階。**先頭から順に試し、合計が収まった時点で止める。**
     *
     * 送っても必ず 400 になるフレームをバッファに積まないためのガードであり、
     * 通常は第1段階で収まる（実効 250〜450KB）。上限が routine な制約ではなく
     * 安全網として機能する状態を保つ。
     */
    val MAIN_STEPS = listOf(
        EncodeStep(longestSide = 1600, quality = 80),
        EncodeStep(longestSide = 1600, quality = 65),
        EncodeStep(longestSide = 1600, quality = 50),
        EncodeStep(longestSide = 1280, quality = 50),
    )

    fun fits(imageBytes: Int, thumbnailBytes: Int): Boolean =
        imageBytes.toLong() + thumbnailBytes.toLong() <= MAX_TOTAL_BYTES
}
