import test from 'node:test';
import assert from 'node:assert/strict';
import { favoriteModelFamilies } from './favorites.js';

const models = [
  { id: 'a-1', family: 'Alpha' },
  { id: 'a-2', family: 'Alpha' },
  { id: 'a-3', family: 'Alpha' },
  { id: 'b-1', family: 'Beta' },
  { id: 'b-2', family: 'Beta' },
  { id: 'c-1', family: 'Gamma' },
  { id: 'c-2', family: 'Gamma' },
  { id: 'd-1', family: 'Delta' }
];
const order = (favorites) => favoriteModelFamilies(models, favorites).map((f) => [f.id, f.models.map((m) => m.id)]);

test('手动关注对家族和模型稳定置顶，组内保留原排序而非星标点击顺序', () => {
  assert.deepEqual(order(['c-2', 'b-2', 'c-1']), [
    ['Beta', ['b-2', 'b-1']],
    ['Gamma', ['c-1', 'c-2']],
    ['Alpha', ['a-1', 'a-2', 'a-3']],
    ['Delta', ['d-1']]
  ]);
  assert.deepEqual(order(['a-3', 'a-2'])[0], ['Alpha', ['a-2', 'a-3', 'a-1']]);
});

test('取消关注恢复基准排序，旧模型星标和运行状态均不自动改变关注', () => {
  const baseline = [
    ['Alpha', ['a-1', 'a-2', 'a-3']],
    ['Beta', ['b-1', 'b-2']],
    ['Delta', ['d-1']],
    ['Gamma', ['c-1', 'c-2']]
  ];
  assert.deepEqual(order([]), baseline);
  assert.deepEqual(order(['removed-model']), baseline);
  assert.deepEqual(
    order(['c-2']).map(([family]) => family),
    ['Gamma', 'Alpha', 'Beta', 'Delta']
  );
  assert.deepEqual(order([]), baseline);
  assert.deepEqual(
    models.map((m) => m.id),
    ['a-1', 'a-2', 'a-3', 'b-1', 'b-2', 'c-1', 'c-2', 'd-1']
  );
});
