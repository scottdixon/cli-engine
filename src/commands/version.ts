import { Command } from 'cli-engine-command'
import { cli } from 'cli-ux'

export default class Version extends Command {
  static topic = 'version'
  static description = 'show CLI version'

  async run() {
    cli.log(this.config.userAgent)
  }
}
