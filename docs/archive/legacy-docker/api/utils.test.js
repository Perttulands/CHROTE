const {
  getGroupPriority,
  categorizeSession,
  sortSessions,
} = require('./utils');

describe('categorizeSession', () => {
  test('categorizes hq-* sessions as hq', () => {
    expect(categorizeSession('hq-mayor')).toBe('hq');
    expect(categorizeSession('hq-deacon')).toBe('hq');
    expect(categorizeSession('hq-admin-extra')).toBe('hq');
  });

  test('categorizes main and shell as main', () => {
    expect(categorizeSession('main')).toBe('main');
    expect(categorizeSession('shell')).toBe('main');
  });

  test('categorizes gt-* sessions by rig name', () => {
    expect(categorizeSession('gt-gastown-jack')).toBe('gt-gastown');
    expect(categorizeSession('gt-beads-lizzy')).toBe('gt-beads');
    expect(categorizeSession('gt-rig1-worker')).toBe('gt-rig1');
  });

  test('categorizes gt- with only two parts', () => {
    expect(categorizeSession('gt-solo')).toBe('gt-solo');
  });

  test('categorizes unknown sessions as other', () => {
    expect(categorizeSession('random')).toBe('other');
    expect(categorizeSession('test-session')).toBe('other');
    expect(categorizeSession('myapp')).toBe('other');
  });
});

describe('getGroupPriority', () => {
  test('hq has highest priority (0)', () => {
    expect(getGroupPriority('hq')).toBe(0);
  });

  test('main has second priority (1)', () => {
    expect(getGroupPriority('main')).toBe(1);
  });

  test('gt-* groups have priority 2', () => {
    expect(getGroupPriority('gt-gastown')).toBe(2);
    expect(getGroupPriority('gt-beads')).toBe(2);
    expect(getGroupPriority('gt-rig1')).toBe(2);
  });

  test('other groups have lowest priority (3)', () => {
    expect(getGroupPriority('other')).toBe(3);
    expect(getGroupPriority('random')).toBe(3);
  });
});

describe('sortSessions', () => {
  test('sorts by group priority first', () => {
    const sessions = [
      { name: 'random', group: 'other' },
      { name: 'gt-gastown-jack', group: 'gt-gastown' },
      { name: 'hq-mayor', group: 'hq' },
      { name: 'main', group: 'main' },
    ];

    const sorted = sortSessions(sessions);

    expect(sorted[0].group).toBe('hq');
    expect(sorted[1].group).toBe('main');
    expect(sorted[2].group).toBe('gt-gastown');
    expect(sorted[3].group).toBe('other');
  });

  test('sorts alphabetically within same priority', () => {
    const sessions = [
      { name: 'gt-beads-lizzy', group: 'gt-beads' },
      { name: 'gt-gastown-jack', group: 'gt-gastown' },
      { name: 'gt-alpha-worker', group: 'gt-alpha' },
    ];

    const sorted = sortSessions(sessions);

    expect(sorted[0].group).toBe('gt-alpha');
    expect(sorted[1].group).toBe('gt-beads');
    expect(sorted[2].group).toBe('gt-gastown');
  });

  test('sorts by session name within same group', () => {
    const sessions = [
      { name: 'gt-gastown-zack', group: 'gt-gastown' },
      { name: 'gt-gastown-jack', group: 'gt-gastown' },
      { name: 'gt-gastown-adam', group: 'gt-gastown' },
    ];

    const sorted = sortSessions(sessions);

    expect(sorted[0].name).toBe('gt-gastown-adam');
    expect(sorted[1].name).toBe('gt-gastown-jack');
    expect(sorted[2].name).toBe('gt-gastown-zack');
  });

  test('does not mutate original array', () => {
    const sessions = [
      { name: 'z', group: 'other' },
      { name: 'a', group: 'hq' },
    ];
    const original = [...sessions];

    sortSessions(sessions);

    expect(sessions).toEqual(original);
  });
});
