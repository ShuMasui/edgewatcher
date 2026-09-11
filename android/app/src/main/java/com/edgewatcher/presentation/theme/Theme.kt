package com.edgewatcher.presentation.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp

/**
 * モック（`docs/mocks/native-mocks.html`）の配色をそのまま持ち込む。
 *
 * **明暗の切り替えは持たない。** 屋外に常設した端末を夜間に見に行くことがあり、
 * 白い画面は暗所で眩しく、設置作業の邪魔になる。モックが暗色1本で描かれているのも
 * 同じ理由による。端末の設定に追随させると、モックと突き合わせて確認できなくなる。
 */
object EdgeWatcherColors {
    val Bg = Color(0xFF0E1116)
    val Panel = Color(0xFF161B23)
    val Panel2 = Color(0xFF1D232D)
    val Line = Color(0xFF2A323F)
    val Text = Color(0xFFE6EDF3)
    val Muted = Color(0xFF8B98A9)
    val Accent = Color(0xFFF0B429)

    /** アクセント上に載る文字。モックの #1b1300。 */
    val OnAccent = Color(0xFF1B1300)

    val Ok = Color(0xFF3FB950)
    val Warn = Color(0xFFD29922)
    val Danger = Color(0xFFF85149)
}

private val scheme = darkColorScheme(
    primary = EdgeWatcherColors.Accent,
    onPrimary = EdgeWatcherColors.OnAccent,
    background = EdgeWatcherColors.Bg,
    onBackground = EdgeWatcherColors.Text,
    surface = EdgeWatcherColors.Panel,
    onSurface = EdgeWatcherColors.Text,
    surfaceVariant = EdgeWatcherColors.Panel2,
    onSurfaceVariant = EdgeWatcherColors.Muted,
    outline = EdgeWatcherColors.Line,
    error = EdgeWatcherColors.Danger,
)

/** モックの font-size をそのまま dp ではなく sp に写した型。 */
private val typography = Typography(
    titleMedium = TextStyle(fontSize = 14.sp, fontWeight = FontWeight.SemiBold),
    bodyMedium = TextStyle(fontSize = 12.5.sp, fontWeight = FontWeight.SemiBold),
    bodySmall = TextStyle(fontSize = 11.sp),
    labelLarge = TextStyle(fontSize = 13.sp),
)

@Composable
fun EdgeWatcherTheme(
    @Suppress("UNUSED_PARAMETER") darkTheme: Boolean = isSystemInDarkTheme(),
    content: @Composable () -> Unit,
) = MaterialTheme(colorScheme = scheme, typography = typography, content = content)
