export type WorldGeoJson = {
  type: string;
  features?: WorldGeoJsonFeature[];
};

export type WorldGeoJsonFeature = {
  properties?: {
    name?: string;
  };
  geometry?: {
    type?: 'Polygon' | 'MultiPolygon';
    coordinates?: number[][][] | number[][][][];
  };
};

let worldGeoJsonPromise: Promise<WorldGeoJson> | null = null;

export function loadWorldGeoJson() {
  if (!worldGeoJsonPromise) {
    worldGeoJsonPromise = fetch('/geo/world-geo.json')
      .then(async (response) => {
        if (!response.ok) {
          throw new Error(
            `world geojson request failed: HTTP ${response.status}`,
          );
        }

        const payload = (await response.json()) as WorldGeoJson;
        if (
          payload.type !== 'FeatureCollection' ||
          !Array.isArray(payload.features)
        ) {
          throw new Error('invalid world geojson payload');
        }

        return payload;
      })
      .catch((error) => {
        worldGeoJsonPromise = null;
        throw error;
      });
  }

  return worldGeoJsonPromise;
}
