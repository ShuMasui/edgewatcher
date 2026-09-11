package com.edgewatcher.presentation.component

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import com.edgewatcher.domain.StatusLine
import com.edgewatcher.presentation.theme.EdgeWatcherColors

/** モックの .appbar。マークとアプリ名だけ。**端末名は出さない。** */
@Composable
fun AppBar() {
    Column(modifier = Modifier.fillMaxWidth()) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(start = 18.dp, end = 18.dp, top = 14.dp, bottom = 12.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            Box(
                modifier = Modifier
                    .size(22.dp)
                    .background(
                        brush = Brush.linearGradient(
                            listOf(EdgeWatcherColors.Accent, Color(0xFFC2410C)),
                        ),
                        shape = RoundedCornerShape(6.dp),
                    ),
            )
            Text("EdgeWatcher", style = MaterialTheme.typography.titleMedium)
        }
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .size(1.dp)
                .background(EdgeWatcherColors.Line),
        )
    }
}

/**
 * モックの .status。印・題・詳細の3つ組。
 *
 * 文言は [StatusLine] が組み立てたものをそのまま出す。ここで作り直すと
 * 常駐通知と食い違う。
 */
@Composable
fun StatusCard(line: StatusLine, modifier: Modifier = Modifier) {
    Row(
        modifier = modifier
            .fillMaxWidth()
            .background(EdgeWatcherColors.Panel, RoundedCornerShape(10.dp))
            .border(1.dp, EdgeWatcherColors.Line, RoundedCornerShape(10.dp))
            .padding(horizontal = 13.dp, vertical = 11.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(9.dp),
    ) {
        Box(
            modifier = Modifier
                .size(9.dp)
                .background(
                    color = when (line.tone) {
                        StatusLine.Tone.OK -> EdgeWatcherColors.Ok
                        StatusLine.Tone.WARN -> EdgeWatcherColors.Warn
                        StatusLine.Tone.OFF -> EdgeWatcherColors.Danger
                    },
                    shape = CircleShape,
                ),
        )
        Column {
            Text(line.title, style = MaterialTheme.typography.bodyMedium)
            Text(
                line.detail,
                style = MaterialTheme.typography.bodySmall,
                color = EdgeWatcherColors.Muted,
                modifier = Modifier.padding(top = 2.dp),
            )
        }
    }
}

/** モックの .btn.primary。 */
@Composable
fun PrimaryButton(text: String, modifier: Modifier = Modifier, onClick: () -> Unit) = Button(
    onClick = onClick,
    modifier = modifier.fillMaxWidth(),
    shape = RoundedCornerShape(10.dp),
    colors = ButtonDefaults.buttonColors(
        containerColor = EdgeWatcherColors.Accent,
        contentColor = EdgeWatcherColors.OnAccent,
    ),
    contentPadding = ButtonDefaults.ContentPadding,
) { Text(text, style = MaterialTheme.typography.labelLarge) }

/** モックの .btn（既定）。枠線つきの控えめなボタン。 */
@Composable
fun SecondaryButton(text: String, modifier: Modifier = Modifier, onClick: () -> Unit) = Button(
    onClick = onClick,
    modifier = modifier.fillMaxWidth(),
    shape = RoundedCornerShape(10.dp),
    border = androidx.compose.foundation.BorderStroke(1.dp, EdgeWatcherColors.Line),
    colors = ButtonDefaults.buttonColors(
        containerColor = EdgeWatcherColors.Panel2,
        contentColor = EdgeWatcherColors.Text,
    ),
) { Text(text, style = MaterialTheme.typography.labelLarge) }

/** モックの .btn.ghost.danger。背景を持たず、文字だけ危険色。 */
@Composable
fun GhostDangerButton(text: String, modifier: Modifier = Modifier, onClick: () -> Unit) = Button(
    onClick = onClick,
    modifier = modifier.fillMaxWidth(),
    shape = RoundedCornerShape(10.dp),
    border = androidx.compose.foundation.BorderStroke(1.dp, EdgeWatcherColors.Line),
    colors = ButtonDefaults.buttonColors(
        containerColor = Color.Transparent,
        contentColor = EdgeWatcherColors.Danger,
    ),
) { Text(text, style = MaterialTheme.typography.labelLarge) }
