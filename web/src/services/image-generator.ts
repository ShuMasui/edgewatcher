/**
 * Generates dynamic mock image SVG DataURLs representing sky/ground scenery based on time of day and seed.
 * Matches the logic in docs/mocks/web-mocks.html.
 */
export function generateMockImageSvg(seed = 1, hour = 14): string {
  const isDay = hour >= 6 && hour < 18;
  const skyTop = isDay ? '#7fa8cd' : '#141c2b';
  const skyBottom = isDay ? '#b9cfe0' : '#2b3242';
  const groundColor = isDay ? '#3f4a37' : '#1a1f1a';
  const skyStop = 20 + (seed % 7);

  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 400 300" width="400" height="300">
    <defs>
      <linearGradient id="sky-${seed}-${hour}" x1="0" y1="0" x2="0" y2="1">
        <stop offset="${skyStop}%" stop-color="${skyTop}" />
        <stop offset="58%" stop-color="${skyBottom}" />
      </linearGradient>
    </defs>
    <rect width="400" height="174" fill="url(#sky-${seed}-${hour})" />
    <rect y="174" width="400" height="126" fill="${groundColor}" />
    <!-- Distant hills/scenery details -->
    <path d="M0,174 Q100,${160 + (seed % 10)} 200,174 T400,174 L400,300 L0,300 Z" fill="${isDay ? '#343e2e' : '#141814'}" opacity="0.6"/>
  </svg>`;

  return `data:image/svg+xml;utf8,${encodeURIComponent(svg)}`;
}
