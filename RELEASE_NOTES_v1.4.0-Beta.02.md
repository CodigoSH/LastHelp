**Notices that say what to do, the processor's temperature, and sizes counted in thousands**

### Added
- **A proxy is in front and was not declared** says where to change the setting and what to run, by how LastHelp is installed: the configuration file and `systemctl` on a native installation, the container's environment otherwise. Each line to type has a button that copies it
- **Server details** show the processor's temperature on a machine that has a sensor, and the number of cores beside its use

### Changed
- The dialogs of **About** have a new look: a translucent sheet over a blur, a title over a rule, rows of an even height and a bar with its track visible
- Sizes in **About** are counted in thousands, as the units they are written in, so a machine with 32 GiB of memory reads 33.3 GB and not 31 GB

### Fixed
- **Reached without HTTPS** counts only somebody signed in using LastHelp over plain HTTP from another machine, and is not said while a proxy in front has not been declared: behind one, the connection to LastHelp is plain whatever the visitor used. A proxy's health probe no longer raises it, and the log says where the first one came from
- The button to check for updates sits the same distance under the status as it does where an update card appears
