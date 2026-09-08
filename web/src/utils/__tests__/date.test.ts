import { describe, it, expect } from 'vitest';
import { formatRelativeTime, formatCountdown, formatDateNavLabel, addDays, isSameDay } from '../date';

describe('date utilities', () => {
  const now = new Date('2026-08-24T14:30:00Z');

  it('should format relative time correctly', () => {
    expect(formatRelativeTime(undefined, now)).toBe('—');
    expect(formatRelativeTime(new Date('2026-08-24T14:29:40Z').toISOString(), now)).toBe('たった今');
    expect(formatRelativeTime(new Date('2026-08-24T14:25:00Z').toISOString(), now)).toBe('5分前');
    expect(formatRelativeTime(new Date('2026-08-24T11:30:00Z').toISOString(), now)).toBe('3時間前');
    expect(formatRelativeTime(new Date('2026-08-22T14:30:00Z').toISOString(), now)).toBe('2日前');
  });

  it('should format countdown timer as M:SS', () => {
    expect(formatCountdown(272)).toBe('4:32');
    expect(formatCountdown(5)).toBe('0:05');
    expect(formatCountdown(0)).toBe('0:00');
    expect(formatCountdown(-10)).toBe('0:00');
  });

  it('should format date nav label', () => {
    const today = new Date(2026, 7, 24); // Aug 24 2026
    const yesterday = new Date(2026, 7, 23); // Aug 23 2026 (Sunday)

    expect(formatDateNavLabel(today, today)).toBe('8/24 (今日)');
    expect(formatDateNavLabel(yesterday, today)).toBe('8/23 (日)');
  });

  it('should calculate addDays and isSameDay correctly', () => {
    const d1 = new Date(2026, 7, 24);
    const d2 = addDays(d1, -1);
    expect(d2.getDate()).toBe(23);
    expect(isSameDay(d1, new Date(2026, 7, 24))).toBe(true);
    expect(isSameDay(d1, d2)).toBe(false);
  });
});
