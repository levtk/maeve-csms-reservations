package cmd

import (
	"crypto/x509"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/thoughtworks/maeve-csms/manager/services"
	"os"
	"strings"
)

var validationTrustRoots []string

// validateCmd represents the validate command
var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate a contract certificate",
	Long:  "Takes a list of <emaid>:<pemFile> arguments and validates each using the OCSP validator",
	RunE: func(cmd *cobra.Command, args []string) error {
		var trustRoots []*x509.Certificate
		for _, pemFile := range validationTrustRoots {
			parsedCerts, err := readCertificatesFromPEMFile(pemFile)
			if err != nil {
				return fmt.Errorf("reading certificates from PEM file: %s: %v", pemFile, err)
			}
			trustRoots = append(trustRoots, parsedCerts...)
		}

		validator := services.OnlineCertificateValidationService{
			RootCertificates: trustRoots,
			MaxOCSPAttempts:  3,
		}

		for _, emaidAndPemFile := range args {
			parts := strings.Split(emaidAndPemFile, ":")
			if len(parts) != 2 {
				return fmt.Errorf("input must be list of <emaid>:<pemFile> pairs")
			}
			emaid := parts[0]
			pemFile := parts[1]
			//#nosec G304 - only files specified by the person running the application will be loaded
			pemData, err := os.ReadFile(pemFile)
			if err != nil {
				return fmt.Errorf("reading certificates from PEM file: %s: %v", pemFile, err)
			}
			_, err = validator.ValidatePEMCertificateChain(pemData, emaid)
			if err == nil {
				fmt.Printf("%s: VALID\n", emaid)
			} else {
				fmt.Printf("%s: %v\n", emaid, err)
			}
		}

		return nil
	},
}

func init() {
	contractCmd.AddCommand(validateCmd)

	validateCmd.Flags().StringSliceVar(&validationTrustRoots, "trust-root", []string{},
		"Specify PEM files containing trusted root certificates")
}