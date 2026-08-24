import React, { useRef, useState, useEffect } from 'react';
import { Button } from '../common/Button';

interface ScrubberProps {
  currentIndex: number;
  totalCount: number;
  onChangeIndex: (index: number) => void;
  onJumpToLatest: () => void;
  isPlaying: boolean;
  onTogglePlay: () => void;
  disabled?: boolean;
}

export const Scrubber: React.FC<ScrubberProps> = ({
  currentIndex,
  totalCount,
  onChangeIndex,
  onJumpToLatest,
  isPlaying,
  onTogglePlay,
  disabled = false,
}) => {
  const trackRef = useRef<HTMLDivElement>(null);
  const [isDragging, setIsDragging] = useState(false);

  const percentage = totalCount > 1 ? (currentIndex / (totalCount - 1)) * 100 : 0;

  const calculateIndexFromPointer = (clientX: number) => {
    if (!trackRef.current || totalCount <= 1) return 0;
    const rect = trackRef.current.getBoundingClientRect();
    const pos = Math.max(0, Math.min(1, (clientX - rect.left) / rect.width));
    return Math.round(pos * (totalCount - 1));
  };

  const handlePointerDown = (e: React.PointerEvent) => {
    if (disabled || totalCount <= 1) return;
    setIsDragging(true);
    const newIdx = calculateIndexFromPointer(e.clientX);
    onChangeIndex(newIdx);
  };

  useEffect(() => {
    if (!isDragging) return;

    const handlePointerMove = (e: PointerEvent) => {
      const newIdx = calculateIndexFromPointer(e.clientX);
      onChangeIndex(newIdx);
    };

    const handlePointerUp = () => {
      setIsDragging(false);
    };

    window.addEventListener('pointermove', handlePointerMove);
    window.addEventListener('pointerup', handlePointerUp);

    return () => {
      window.removeEventListener('pointermove', handlePointerMove);
      window.removeEventListener('pointerup', handlePointerUp);
    };
  }, [isDragging, totalCount]);

  return (
    <div className="scrubber-container" data-testid="scrubber">
      <div className="transport">
        <Button
          size="sm"
          onClick={onTogglePlay}
          disabled={disabled || totalCount <= 1}
          aria-label={isPlaying ? '一時停止' : '再生'}
          data-testid="play-pause-btn"
        >
          {isPlaying ? '❚❚' : '▶'}
        </Button>

        <div
          className="track-container"
          ref={trackRef}
          onPointerDown={handlePointerDown}
          data-testid="scrubber-track"
        >
          <div className="track">
            <div className="fill" style={{ width: `${percentage}%` }} />
            <div
              className="knob"
              style={{ left: `${percentage}%` }}
              role="slider"
              aria-valuenow={currentIndex}
              aria-valuemin={0}
              aria-valuemax={Math.max(0, totalCount - 1)}
              data-testid="scrubber-knob"
            />
          </div>
        </div>

        <button
          className="btn sm"
          onClick={onJumpToLatest}
          disabled={disabled || currentIndex === totalCount - 1}
          data-testid="jump-latest-btn"
        >
          <span className="num">最新へ</span>
        </button>
      </div>

      <div className="ticks">
        <span>0:00</span>
        <span>6:00</span>
        <span>12:00</span>
        <span>18:00</span>
        <span>今</span>
      </div>
    </div>
  );
};
