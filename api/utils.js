// Session utility functions
// Extracted for testability

// Session sorting priority by group
const GROUP_PRIORITY = {
  'hq': 0,
  'main': 1,
};

function getGroupPriority(group) {
  if (GROUP_PRIORITY[group] !== undefined) return GROUP_PRIORITY[group];
  if (group.startsWith('gt-')) return 2; // Rigs after main
  return 3; // Other
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

function extractAgentName(sessionName) {
  // gt-gastown-jack → jack
  // hq-mayor → mayor
  const parts = sessionName.split('-');
  if (parts.length >= 3 && sessionName.startsWith('gt-')) {
    return parts.slice(2).join('-');
  }
  if (parts.length >= 2) {
    return parts.slice(1).join('-');
  }
  return sessionName;
}

function sortSessions(sessions) {
  return [...sessions].sort((a, b) => {
    const priorityA = getGroupPriority(a.group);
    const priorityB = getGroupPriority(b.group);
    if (priorityA !== priorityB) return priorityA - priorityB;
    if (a.group !== b.group) return a.group.localeCompare(b.group);
    return a.agentName.localeCompare(b.agentName);
  });
}

module.exports = {
  GROUP_PRIORITY,
  getGroupPriority,
  categorizeSession,
  extractAgentName,
  sortSessions,
};
