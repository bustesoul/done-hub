export const groupAvailableModels = (models = {}, ownedBy = [], fallbackProvider = 'Unknown') => {
  const providerOrder = new Map((Array.isArray(ownedBy) ? ownedBy : []).map((provider, index) => [provider.name, provider.id ?? index]));
  const grouped = new Map();

  Object.entries(models || {}).forEach(([model, info]) => {
    const provider = info?.owned_by || fallbackProvider;
    if (!grouped.has(provider)) grouped.set(provider, []);
    grouped.get(provider).push(model);
  });

  return [...grouped.entries()]
    .sort(([providerA], [providerB]) => {
      const orderA = providerOrder.get(providerA) ?? Number.MAX_SAFE_INTEGER;
      const orderB = providerOrder.get(providerB) ?? Number.MAX_SAFE_INTEGER;
      return orderA - orderB || providerA.localeCompare(providerB);
    })
    .map(([provider, providerModels]) => ({
      provider,
      models: providerModels.sort((a, b) => a.localeCompare(b))
    }));
};

export const flattenAvailableModelGroups = (groups) =>
  groups.flatMap(({ provider, models }) => models.map((model) => ({ id: model, group: provider })));
