import React from 'react';
import { Observation } from '../../types/domain';

interface ThumbnailStripProps {
  observations: Observation[];
  currentIndex: number;
  onSelectIndex: (index: number) => void;
  isEmptyDay: boolean;
}

export const ThumbnailStrip: React.FC<ThumbnailStripProps> = ({
  observations,
  currentIndex,
  onSelectIndex,
  isEmptyDay,
}) => {
  if (isEmptyDay || observations.length === 0) {
    return <div className="empty-day" data-testid="empty-strip">表示できる画像がありません</div>;
  }

  // Display window of up to 7 items around currentIndex
  const total = observations.length;
  const windowSize = 7;
  let start = Math.max(0, currentIndex - Math.floor(windowSize / 2));
  const end = Math.min(total, start + windowSize);

  if (end - start < windowSize) {
    start = Math.max(0, end - windowSize);
  }

  const visibleObservations = observations.slice(start, end);

  return (
    <div className="strip-frame">
      <div className="strip" data-testid="thumbnail-strip">
        {visibleObservations.map((obs, idx) => {
          const actualIndex = start + idx;
          const isSelected = actualIndex === currentIndex;

          return (
            <button
              key={obs.observationId}
              type="button"
              className={`cell ${isSelected ? 'sel' : ''}`}
              onClick={() => onSelectIndex(actualIndex)}
              aria-label={`撮影時刻 ${obs.capturedAt}`}
              aria-current={isSelected}
              data-testid={`thumb-${actualIndex}`}
            >
              <img src={obs.thumbnailUrl} alt="" loading="lazy" />
            </button>
          );
        })}
      </div>
    </div>
  );
};
