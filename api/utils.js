// Session utility functions
// Extracted for testability

// Session sorting priority by group
const GROUP_PRIORITY = {
  'hq': 0,
  'main': 1,
};

function getGroupPriority(group) {
  if (GROUP_PRIORITY[group] !== undefined) return GROUP_PRIORITY[group];
  if (group.startsWith('gt-')) return 3; // Rigs after main
  return 4; // Other
}

function categorizeSession(name) {
  if (name.startsWith('hq-')) return 'hq';
  if (name === 'main' || name === 'shell') return 'main';
  if (name.startsWith('gt-')) {
    // Extract rig name: gt-gastown-jack → gt-gastown
    const parts = name.split('-');
    if (parts.length >= 2) {
      return parts.slice(0, 2).join('-');
    }
    return 'gt-unknown';
  }
  return 'other';
}

function sortSessions(sessions) {
  return [...sessions].sort((a, b) => {
    const priorityA = getGroupPriority(a.group);
    const priorityB = getGroupPriority(b.group);
    if (priorityA !== priorityB) return priorityA - priorityB;
    if (a.group !== b.group) return a.group.localeCompare(b.group);
    return a.name.localeCompare(b.name);
  });
}

module.exports = {
  GROUP_PRIORITY,
  getGroupPriority,
  categorizeSession,
  sortSessions,
};
