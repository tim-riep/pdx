<?xml version="1.0" encoding="UTF-8"?>
<xsl:stylesheet version="3.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform" xmlns:fo="http://www.w3.org/1999/XSL/Format" xmlns:pdx="https://github.com/tim-riep/pdx/ns/fo/1" xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:a="https://github.com/tim-riep/pdx/ns/examples/antrag" exclude-result-prefixes="xs a">
  <!-- Erzeugt von pdx aus der Vorlage formular. Nicht von Hand bearbeiten. -->
  <xsl:output method="xml" encoding="UTF-8" indent="no"/>
  <xsl:template match="/">
    <xsl:apply-templates select="*"/>
  </xsl:template>
  <xsl:template match="/*">
    <xsl:message terminate="yes">
      <xsl:text>Die Daten haben das Wurzelelement </xsl:text>
      <xsl:value-of select="name()"/>
      <xsl:text>, erwartet ist a:antrag.</xsl:text>
    </xsl:message>
  </xsl:template>
  <xsl:template match="/a:antrag">
    <fo:root pdx:id="formular" xml:lang="de-DE">
      <fo:layout-master-set pdx:id="seiten">
        <fo:simple-page-master pdx:id="seite" master-name="seite" page-width="210mm" page-height="297mm" margin="20mm">
          <fo:region-body pdx:id="seite"/>
        </fo:simple-page-master>
        <fo:page-sequence-master pdx:id="seiten" master-name="seiten">
          <fo:repeatable-page-master-alternatives pdx:id="seiten">
            <fo:conditional-page-master-reference pdx:id="seite" master-reference="seite"/>
          </fo:repeatable-page-master-alternatives>
        </fo:page-sequence-master>
      </fo:layout-master-set>
      <fo:declarations>
        <pdx:metadata title="Antrag (Formular)"/>
      </fo:declarations>
      <fo:page-sequence pdx:id="seiten" id="pdx-sequence" master-reference="seiten" font-family="Helvetica" font-size="10pt" line-height="1.4">
        <fo:flow pdx:id="inhalt" flow-name="xsl-region-body">
          <fo:block pdx:id="titel" role="H1" font-family="Helvetica" font-size="16pt" font-weight="bold" line-height="1.4" space-after="6mm">
            <xsl:text>Antrag</xsl:text>
          </fo:block>
          <fo:block pdx:id="anrede-text" space-after="1mm">
            <xsl:text>Anrede</xsl:text>
          </fo:block>
          <fo:block pdx:id="anrede">
            <fo:block>
              <fo:instream-foreign-object pdx:id="anrede">
                <pdx:form-field type="radio" name="anrede" label="Anrede" export="Frau">
                  <xsl:attribute name="value" select="string(a:anrede[1])"/>
                  <xsl:attribute name="context" select="path(.)"/>
                </pdx:form-field>
              </fo:instream-foreign-object>
              <fo:inline padding-left="0.4em">
                <xsl:text>Frau</xsl:text>
              </fo:inline>
            </fo:block>
            <fo:block>
              <fo:instream-foreign-object pdx:id="anrede">
                <pdx:form-field type="radio" name="anrede" label="Anrede" export="Herr">
                  <xsl:attribute name="value" select="string(a:anrede[1])"/>
                  <xsl:attribute name="context" select="path(.)"/>
                </pdx:form-field>
              </fo:instream-foreign-object>
              <fo:inline padding-left="0.4em">
                <xsl:text>Herr</xsl:text>
              </fo:inline>
            </fo:block>
            <fo:block>
              <fo:instream-foreign-object pdx:id="anrede">
                <pdx:form-field type="radio" name="anrede" label="Anrede" export="divers">
                  <xsl:attribute name="value" select="string(a:anrede[1])"/>
                  <xsl:attribute name="context" select="path(.)"/>
                </pdx:form-field>
              </fo:instream-foreign-object>
              <fo:inline padding-left="0.4em">
                <xsl:text>divers</xsl:text>
              </fo:inline>
            </fo:block>
          </fo:block>
          <fo:block pdx:id="name-text" space-before="3mm">
            <xsl:text>Name</xsl:text>
          </fo:block>
          <fo:block pdx:id="name" font-family="Helvetica" font-size="10pt" line-height="1.4">
            <fo:instream-foreign-object pdx:id="name" content-width="90mm" content-height="7mm" border="0.5pt solid #555555">
              <pdx:form-field type="text" name="name" label="Name" max-length="40" required="true">
                <xsl:attribute name="value" select="string(a:name[1])"/>
                <xsl:attribute name="context" select="path(.)"/>
              </pdx:form-field>
            </fo:instream-foreign-object>
          </fo:block>
          <fo:block pdx:id="email-text" space-before="3mm">
            <xsl:text>E-Mail</xsl:text>
          </fo:block>
          <fo:block pdx:id="email" font-family="Helvetica" font-size="10pt" line-height="1.4">
            <fo:instream-foreign-object pdx:id="email" content-width="90mm" content-height="7mm" border="0.5pt solid #555555">
              <pdx:form-field type="text" name="email" label="E-Mail-Adresse">
                <xsl:attribute name="value" select="string(a:email[1])"/>
                <xsl:attribute name="context" select="path(.)"/>
              </pdx:form-field>
            </fo:instream-foreign-object>
          </fo:block>
          <fo:block pdx:id="land-text" space-before="3mm">
            <xsl:text>Land</xsl:text>
          </fo:block>
          <fo:block pdx:id="land" font-family="Helvetica" font-size="10pt" line-height="1.4">
            <fo:instream-foreign-object pdx:id="land" content-width="40mm" content-height="7mm" border="0.5pt solid #555555">
              <pdx:form-field type="combo" name="land" label="Land">
                <xsl:attribute name="value" select="string(a:land[1])"/>
                <xsl:attribute name="context" select="path(.)"/>
                <pdx:option value="DE" label="Deutschland"/>
                <pdx:option value="AT" label="Österreich"/>
                <pdx:option value="CH" label="Schweiz"/>
              </pdx:form-field>
            </fo:instream-foreign-object>
          </fo:block>
          <fo:block pdx:id="tarif-text" space-before="3mm">
            <xsl:text>Tarif</xsl:text>
          </fo:block>
          <fo:block pdx:id="tarif" font-family="Helvetica" font-size="10pt" line-height="1.4">
            <fo:instream-foreign-object pdx:id="tarif" content-width="40mm" content-height="16mm" border="0.5pt solid #555555">
              <pdx:form-field type="list" name="tarif" label="Tarif">
                <xsl:attribute name="value" select="string(a:tarif[1])"/>
                <xsl:attribute name="context" select="path(.)"/>
                <pdx:option value="Basis" label="Basis"/>
                <pdx:option value="Komfort" label="Komfort"/>
                <pdx:option value="Premium" label="Premium"/>
              </pdx:form-field>
            </fo:instream-foreign-object>
          </fo:block>
          <fo:block pdx:id="newsletter-text" space-before="3mm">
            <xsl:text>Newsletter abonnieren</xsl:text>
          </fo:block>
          <fo:block pdx:id="newsletter" font-family="Helvetica" font-size="10pt" line-height="1.4">
            <fo:instream-foreign-object pdx:id="newsletter" content-width="5mm" content-height="5mm" border="0.5pt solid #555555">
              <pdx:form-field type="checkbox" name="newsletter" label="Newsletter abonnieren" on-value="true" off-value="false">
                <xsl:attribute name="value" select="string(a:newsletter[1])"/>
                <xsl:attribute name="context" select="path(.)"/>
              </pdx:form-field>
            </fo:instream-foreign-object>
          </fo:block>
          <fo:block pdx:id="bemerkung-text" space-before="3mm">
            <xsl:text>Bemerkung</xsl:text>
          </fo:block>
          <fo:block pdx:id="bemerkung" font-family="Helvetica" font-size="10pt" line-height="1.4">
            <fo:instream-foreign-object pdx:id="bemerkung" content-width="150mm" content-height="20mm" border="0.5pt solid #555555">
              <pdx:form-field type="text" name="bemerkung" label="Bemerkung" multiline="true">
                <xsl:attribute name="value" select="string(a:bemerkung[1])"/>
                <xsl:attribute name="context" select="path(.)"/>
              </pdx:form-field>
            </fo:instream-foreign-object>
          </fo:block>
          <fo:block pdx:id="personen-text" role="H2" space-before="5mm" space-after="2mm">
            <xsl:text>Personen</xsl:text>
          </fo:block>
          <xsl:for-each select="a:personen/a:person">
            <fo:block pdx:id="person" space-after="2mm">
              <fo:block pdx:id="vorname" font-family="Helvetica" font-size="10pt" line-height="1.4">
                <fo:instream-foreign-object pdx:id="vorname" content-width="60mm" content-height="7mm" border="0.5pt solid #555555">
                  <pdx:form-field type="text" name="vorname" label="Vorname">
                    <xsl:attribute name="value" select="string(a:vorname[1])"/>
                    <xsl:attribute name="context" select="path(.)"/>
                  </pdx:form-field>
                </fo:instream-foreign-object>
              </fo:block>
            </fo:block>
          </xsl:for-each>
          <fo:block pdx:id="unterschrift-text" space-before="6mm">
            <xsl:text>Unterschrift</xsl:text>
          </fo:block>
          <fo:block pdx:id="unterschrift">
            <fo:instream-foreign-object pdx:id="unterschrift" content-width="70mm" content-height="15mm">
              <pdx:form-field type="signature" name="unterschrift" label="Unterschrift">
              </pdx:form-field>
            </fo:instream-foreign-object>
          </fo:block>
        </fo:flow>
      </fo:page-sequence>
    </fo:root>
  </xsl:template>
</xsl:stylesheet>
