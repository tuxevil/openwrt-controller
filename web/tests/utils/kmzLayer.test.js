import { describe, it, expect } from 'vitest'
import { selectKmzTilesForBox } from '../../src/utils/kmzLayer'

const altavistaBounds = [
  [-2.025369, -78.951177],
  [-2.019919, -78.945526],
]

describe('selectKmzTilesForBox', () => {
  it('returns null when viewport tile does not intersect KMZ bounds', () => {
    const res = selectKmzTilesForBox(altavistaBounds, 0, 6, {
      south: -1.5,
      north: -1.4,
      west: -78.5,
      east: -78.4,
    })
    expect(res).toBeNull()
  })

  it('picks root level 0 when zoomed far out covering whole orthophoto', () => {
    const res = selectKmzTilesForBox(altavistaBounds, 0, 6, {
      south: -2.04,
      north: -2.01,
      west: -78.97,
      east: -78.93,
    })
    expect(res).not.toBeNull()
    expect(res.level).toBe(0)
    expect(res.tiles).toHaveLength(1)
    expect(res.tiles[0]).toMatchObject({ z: 0, x: 0, y: 0 })
  })

  it('picks fine level 6 and exact tile 6/43/18 around the house at high zoom', () => {
    const res = selectKmzTilesForBox(altavistaBounds, 0, 6, {
      south: -2.02382,
      north: -2.02376,
      west: -78.94736,
      east: -78.94730,
    })
    expect(res).not.toBeNull()
    expect(res.level).toBe(6)
    expect(res.tiles.some((t) => t.z === 6 && t.x === 43 && t.y === 18)).toBe(true)
  })
})
