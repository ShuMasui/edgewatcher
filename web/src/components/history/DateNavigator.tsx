import React from 'react';
import { formatDateNavLabel } from '../../utils/date';

interface DateNavigatorProps {
  currentDate: Date;
  atOldest: boolean;
  atNewest: boolean;
  onPrevDate: () => void;
  onNextDate: () => void;
}

export const DateNavigator: React.FC<DateNavigatorProps> = ({
  currentDate,
  atOldest,
  atNewest,
  onPrevDate,
  onNextDate,
}) => {
  return (
    <div className="datenav" data-testid="date-navigator">
      <button
        className="arrow"
        disabled={atOldest}
        onClick={onPrevDate}
        aria-label="前の日へ"
        data-testid="prev-date-btn"
      >
        &lsaquo;
      </button>
      <span className="day" data-testid="date-label">
        {formatDateNavLabel(currentDate)}
      </span>
      <button
        className="arrow"
        disabled={atNewest}
        onClick={onNextDate}
        aria-label="次の日へ"
        data-testid="next-date-btn"
      >
        &rsaquo;
      </button>
    </div>
  );
};
