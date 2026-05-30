// Exact port of pkg/tower.GroupColor so a group's color is identical across the
// TUI and the web client. The town group gets the reserved amber accent; every
// other group (a rig) is assigned a palette hue by an FNV-1a hash of its name.
//
// fnv1a mirrors Go's hash/fnv.New32a over the name's bytes. Rig/group names are
// ASCII, so charCodeAt is byte-accurate.

const TOWN_COLOR = '#e8a33d';

const RIG_PALETTE = [
  '#3da5e8', // blue
  '#5ad17a', // green
  '#c77dff', // violet
  '#ff7d9c', // pink
  '#4fd6c8', // teal
  '#ff8c42', // orange
  '#9d8cff', // indigo
  '#e8d44d', // gold
];

function fnv1a(str) {
  let h = 0x811c9dc5; // FNV offset basis (32-bit)
  for (let i = 0; i < str.length; i++) {
    h ^= str.charCodeAt(i);
    h = Math.imul(h, 0x01000193) >>> 0; // FNV prime, kept unsigned 32-bit
  }
  return h >>> 0;
}

export const TOWN_GROUP = 'town';

export function groupColor(group) {
  if (group === TOWN_GROUP) return TOWN_COLOR;
  return RIG_PALETTE[fnv1a(group) % RIG_PALETTE.length];
}
