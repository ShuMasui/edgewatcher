import React, { useState, useEffect } from 'react';
import { Observation } from '../../types/domain';
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

  return (
    <div className="viewer" data-testid="history-viewer">
      {isEmptyDay ? (
        <div
          className="placeholder"
          style={{ aspectRatio: '4/3', borderRadius: 'var(--radius)' }}
          data-testid="empty-day-placeholder"
        >
          この日、この端末からは画像が届いていません
        </div>
      ) : (
        <div className="big" data-testid="main-image-container">
          {currentObservation && (
            <>
              <img
                src={currentObservation.imageUrl || currentObservation.thumbnailUrl}
                alt={`観測画像 ${currentObservation.capturedAt}`}
                onError={onImageError}
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
