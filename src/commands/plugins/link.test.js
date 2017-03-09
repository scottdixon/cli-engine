// @flow

import Link from './link'
import Index from './index'
import Uninstall from './uninstall'

beforeEach(() => {
  jasmine.DEFAULT_TIMEOUT_INTERVAL = 30000
})

test('links example plugin', async () => {
  let index = await Index.run([], {mock: true})
  if (index.stdout.output.includes('cli-engine-example-plugin')) {
    await Uninstall.run(['cli-engine-example-plugin'], {mock: true})
  }
  await Link.run(['./example-plugin'], {mock: true})
  index = await Index.run([], {mock: true})
  expect(index.stdout.output).toContain('cli-engine-example-plugin')
})
