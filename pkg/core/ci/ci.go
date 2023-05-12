package ci

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/carbonetes/diggity/pkg/convert"
	dm "github.com/carbonetes/diggity/pkg/model"
	diggity "github.com/carbonetes/diggity/pkg/scanner"
	"github.com/carbonetes/jacked/internal/logger"
	save "github.com/carbonetes/jacked/internal/output/save"
	jacked "github.com/carbonetes/jacked/pkg/core/analysis"
	"github.com/carbonetes/jacked/pkg/core/ci/assessment"
	"github.com/carbonetes/jacked/pkg/core/ci/table"
	"github.com/carbonetes/jacked/pkg/core/model"
	"github.com/logrusorgru/aurora"
	"golang.org/x/exp/slices"
)

var (
	log                    = logger.GetLogger()
	defaultCriteria string = "LOW"
)

func Analyze(args *model.Arguments) {
	var outputText string
	log.Println(aurora.Blue("Entering CI Mode...\n").String())
	if args.FailCriteria == nil || len(*args.FailCriteria) == 0 || !slices.Contains(assessment.Severities, strings.ToUpper(*args.FailCriteria)) {
		warningMessage := fmt.Sprintf("Invalid criteria specified : %v\nSet to default criteria : %v", *args.FailCriteria, defaultCriteria)
		log.Warnf(warningMessage)
		outputText = warningMessage
		args.FailCriteria = &defaultCriteria
	}
	diggityArgs := dm.NewArguments()
	if len(*args.Image) > 0 {
		imageInfo := fmt.Sprintf("\n\nImage: %s", *args.Image)
		log.Printf(imageInfo)
		outputText += imageInfo + "\n\n"
		diggityArgs.Image = args.Image
		diggityArgs.RegistryUsername = args.RegistryUsername
		diggityArgs.RegistryPassword = args.RegistryPassword
		diggityArgs.RegistryURI = args.RegistryURI
		diggityArgs.RegistryToken = args.RegistryToken
	} else if len(*args.Dir) > 0 {
		dirInfo := fmt.Sprintf("\tDir: %6s\n", *args.Dir)
		log.Printf(dirInfo)
		outputText += dirInfo + "\n\n"
		diggityArgs.Dir = args.Dir
	} else if len(*args.Tar) > 0 {
		tarInfo := fmt.Sprintf("\tTar: %6s\n", *args.Tar)
		log.Printf(tarInfo)
		outputText += tarInfo + "\n\n"
		diggityArgs.Tar = args.Tar
	} else {
		log.Fatalf("No valid scan target specified!")
	}
	log.Println(aurora.Blue("\nGenerating CDX BOM...\n"))
	sbom, _ := diggity.Scan(diggityArgs)

	if sbom.Packages == nil {
		log.Error("No package found to analyze!")
	}

	cdx := convert.ToCDX(sbom.Packages)

	outputText += "Generated CDX BOM\n\n" + table.CDXBomTable(cdx)

	log.Println(aurora.Blue("\nAnalyzing CDX BOM...\n").String())
	jacked.AnalyzeCDX(cdx)

	if len(*cdx.Vulnerabilities) == 0 {
		fmt.Println("No vulnerabilities found!")
		outputText += "\nNo vulnerabilities found! \n"
	} else {
		outputText += "\n\nAnalyzed CDX BOM \n\n" + table.CDXVexTable(cdx)
	}

	stats := fmt.Sprintf("\nPackages: %9v\nVulnerabilities: %v", len(*cdx.Components), len(*cdx.Vulnerabilities))
	outputText += "\n" + stats
	log.Println(aurora.Cyan(stats).String())
	log.Println(aurora.Blue("\nExecuting CI Assessment...\n").String())

	log.Println(aurora.Blue("\nAssessment Result:\n").String())
	outputText += "\n\nAssessment Result:\n"
	if len(*cdx.Vulnerabilities) == 0 {
		message := fmt.Sprintf("\nPassed: %5v found components\n", len(*cdx.Components))
		outputText += message
		log.Println(aurora.Green(aurora.Bold(message).String()))
	}

	result := assessment.Evaluate(args.FailCriteria, cdx)

	outputText += "\n"+table.TallyTable(result.Tally)
	outputText += "\n"+table.MatchTable(result.Matches)
	for _, m := range *result.Matches {
		if len(m.Vulnerability.Recommendation) > 0 {
			recMessage := fmt.Sprintf("[%v] : %v", m.Vulnerability.ID, m.Vulnerability.Recommendation)
			outputText += "\n" + recMessage
			log.Warnf(recMessage)
		}
	}
	totalVulnerabilities := len(*cdx.Vulnerabilities)
	if result.Passed {
		passedMessage := fmt.Sprintf("\nPassed: %5v out of %v found vulnerabilities passed the assessment\n", totalVulnerabilities, totalVulnerabilities)
		outputText += "\n" + passedMessage
		log.Println(aurora.Green(aurora.Bold(passedMessage).String()))
		os.Exit(0)
	}
	failedMessage := fmt.Sprintf("\nFailed: %5v out of %v found vulnerabilities failed the assessment \n", len(*result.Matches), totalVulnerabilities)
	outputText += "\n" + failedMessage
	log.Error(errors.New(aurora.Red(aurora.Bold(failedMessage).String()).String()))
	
	if args.OutputFile != nil && *args.OutputFile != ""{
		// we can use the *args.Output for the second args on the parameter, for now it only supports table/txt output
		save.SaveOutputAsFile(*args.OutputFile,"table", outputText )
		
	}
	
	os.Exit(1)
}
