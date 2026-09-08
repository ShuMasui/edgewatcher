import React, { useState, useEffect } from 'react';
import { Observation } from '../../types/domain';
import { useObservationImage } from '../../hooks/use-observations';
import { formatDateTime } from '../../utils/date';
import { Scrubber } from './Scrubber';
import { ThumbnailStrip } from './ThumbnailStrip';

interface HistoryViewerProps {
  observations: Observation[];
  retentionDays: number;
  atOldest: boolean;
  onImageError: () => void;
}

export const HistoryViewer: React.FC<HistoryViewerProps> = ({
  observations,
  retentionDays,
  atOldest,
  onImageError,
}) => {
  const [currentIndex, setCurrentIndex] = useState<number>(0);
  const [isPlaying, setIsPlaying] = useState<boolean>(false);

  // Set initial position to the latest observation of the day
  useEffect(() => {
    if (observations.length > 0) {
      setCurrentIndex(observations.length - 1);
    } else {
      setCurrentIndex(0);
    }
    setIsPlaying(false);
  }, [observations]);

  // Timelapse playback loop
  useEffect(() => {
    if (!isPlaying || observations.length <= 1) return;

    const interval = setInterval(() => {
      setCurrentIndex((prev) => {
        if (prev >= observations.length - 1) {
          return 0; // Loop back to start
        }
        return prev + 1;
      });
    }, 600);

    return () => clearInterval(interval);
  }, [isPlaying, observations.length]);

  const isEmptyDay = observations.length === 0;
  const currentObservation = observations[currentIndex] || null;

  // GET /devices/{id}/observations は thumbnailUrl だけを返し、本画像の
  // 署名付き URL は付けない(05-backend.md §1.5)。1日分すべてに本画像の
  // URL を署名すると、ブラウザは開きもしない画像の有効な認可を大量に
  // 受け取ることになるため。表示中の1枚だけをここで都度取得する。
  //
  // モックは一覧に imageUrl を載せるので、その場合はこの取得は走らない
  // (enabled が false になるわけではなく、下の優先順で使われない)。
  const { imageData: fullImage, handleImageError: onFullImageError } = useObservationImage(
    currentObservation && !currentObservation.imageUrl ? currentObservation.observationId : null,
    currentObservation && !currentObservation.imageUrl ? currentObservation.deviceId : null
  );

  // 一覧が本画像を持っていればそれを使い、無ければ都度取得したものを使い、
  // それも無ければサムネイルで代替する。最後の段があるのは、署名の取得が
  // 終わるまでの一瞬と、取得に失敗したときに画面を空にしないため。
  const displayedImageUrl =
    currentObservation?.imageUrl || fullImage?.imageUrl || currentObservation?.thumbnailUrl;

  return (
    <div className="viewer" data-testid="history-viewer">
      {isEmptyDay ? (
        <div className="placeholder" data-testid="empty-day-placeholder">
          <span>この日、この端末からは画像が届いていません</span>
        </div>
      ) : (
        <div className="big" data-testid="main-image-container">
          {currentObservation && (
            <>
              <img
                src={displayedImageUrl}
                alt={`観測画像 ${currentObservation.capturedAt}`}
                onError={() => {
                  // 署名付き URL の期限切れは、一覧側と本画像側のどちらでも
                  // 起こりうる(03-web.md §1.10.5)。どちらが切れたのかは
                  // img の onError からは分からないので両方を無効化する。
                  onFullImageError();
                  onImageError();
                }}
                data-testid="main-history-image"
              />
              <span className="stamp">{formatDateTime(currentObservation.capturedAt)}</span>
            </>
          )}
        </div>
      )}

      <Scrubber
        currentIndex={currentIndex}
        totalCount={observations.length}
        onChangeIndex={(idx) => {
          setCurrentIndex(idx);
          setIsPlaying(false);
        }}
        onJumpToLatest={() => {
          setCurrentIndex(Math.max(0, observations.length - 1));
          setIsPlaying(false);
        }}
        isPlaying={isPlaying}
        onTogglePlay={() => setIsPlaying((prev) => !prev)}
        disabled={isEmptyDay}
      />

      <ThumbnailStrip
        observations={observations}
        currentIndex={currentIndex}
        onSelectIndex={(idx) => {
          setCurrentIndex(idx);
          setIsPlaying(false);
        }}
        isEmptyDay={isEmptyDay}
      />

      {atOldest && (
        <div className="retention-note" data-testid="retention-note">
          これより前の画像は保持期間({retentionDays}日)を過ぎています
        </div>
      )}
    </div>
  );
};
