# CYP / P450 database

Selecting `CYP / P450 keyword` checks the independent `phytozome-go-p450phgo` release repository for `p450phgo.pgd`. The file is downloaded or replaced beside the application before species selection. If the download or validation fails, the workflow returns to the default Phytozome database.

The published database is deliberately conservative. Every Dr Nelson resource (40 animal, 59 plant, 61 fungal and 7 bacterial resources) has a downloaded source and an audit record in the generator repository. A resource is not searchable until its own parser establishes the species-to-record relationship; disabled species remain visible so missing coverage is explicit. The current release contains the reviewed CAld5H/CYP84 Table S2 relationships and does not promote generic binary-string matches.

Search is local after download. Species are read from the PGD and no online page is queried. Category prefixes `a:`, `p:`, `f:`, and `b:` (or the full category names) restrict a keyword to animals, plants, fungi, or bacteria.

The Species mode has three choices. Custom assigns a species per row; a blank row inherits the nearest earlier row with a species, while a blank first row is rejected. Set all hides the row Species column and assigns one selected species to every row. Set remaining preserves explicit row species and fills only blank rows with the selected fallback species. In Custom mode, focus a Species cell and press Space or Enter to open the local species list.
