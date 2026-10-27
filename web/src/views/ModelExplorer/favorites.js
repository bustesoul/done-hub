import { groupAvailableModels } from '../../utils/availableModels.js';

export function favoriteModelFamilies(models, favorites) {
  const favoriteIDs = new Set(favorites);
  const favoriteFamilies = new Set(models.filter((m) => favoriteIDs.has(m.id)).map((m) => m.family));
  return groupAvailableModels(Object.fromEntries(models.map((m) => [m.id, { owned_by: m.family }])))
    .map(({ provider }) => ({
      id: provider,
      name: provider,
      models: models.filter((m) => m.family === provider).sort((a, b) => Number(favoriteIDs.has(b.id)) - Number(favoriteIDs.has(a.id)))
    }))
    .sort((a, b) => Number(favoriteFamilies.has(b.id)) - Number(favoriteFamilies.has(a.id)));
}
