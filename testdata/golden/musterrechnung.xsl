<?xml version="1.0" encoding="UTF-8"?>
<xsl:stylesheet version="3.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform" xmlns:fo="http://www.w3.org/1999/XSL/Format" xmlns:pdx="https://pdx.example/ns/pdx/1.0" xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:inv="https://pdx.example/ns/invoice/1" exclude-result-prefixes="xs inv">
  <!-- Erzeugt von pdx aus der Vorlage musterrechnung. Nicht von Hand bearbeiten. -->
  <xsl:output method="xml" encoding="UTF-8" indent="no"/>
  <xsl:decimal-format name="fmt-de" decimal-separator="," grouping-separator="."/>
  <xsl:template match="/">
    <xsl:apply-templates select="*"/>
  </xsl:template>
  <xsl:template match="/*">
    <xsl:message terminate="yes">
      <xsl:text>Die Daten haben das Wurzelelement </xsl:text>
      <xsl:value-of select="name()"/>
      <xsl:text>, erwartet ist inv:invoice.</xsl:text>
    </xsl:message>
  </xsl:template>
  <xsl:template match="/inv:invoice">
    <fo:root pdx:id="musterrechnung" xml:lang="de-DE">
      <fo:layout-master-set pdx:id="seiten">
        <fo:simple-page-master pdx:id="erste" master-name="erste" page-width="210mm" page-height="297mm" margin="15mm 20mm 15mm 25mm">
          <fo:region-body pdx:id="erste" margin-top="90mm" margin-bottom="17mm"/>
          <fo:region-before pdx:id="erste-kopf" region-name="erste-kopf" extent="85mm"/>
          <fo:region-after pdx:id="erste-fuss" region-name="erste-fuss" extent="12mm"/>
        </fo:simple-page-master>
        <fo:simple-page-master pdx:id="folge" master-name="folge" page-width="210mm" page-height="297mm" margin="15mm 20mm 15mm 25mm">
          <fo:region-body pdx:id="folge" margin-top="15mm" margin-bottom="17mm"/>
          <fo:region-before pdx:id="folge-kopf" region-name="folge-kopf" extent="10mm"/>
          <fo:region-after pdx:id="folge-fuss" region-name="folge-fuss" extent="12mm"/>
        </fo:simple-page-master>
        <fo:page-sequence-master pdx:id="seiten" master-name="seiten">
          <fo:repeatable-page-master-alternatives pdx:id="seiten">
            <fo:conditional-page-master-reference pdx:id="erste" master-reference="erste" page-position="first"/>
            <fo:conditional-page-master-reference pdx:id="folge" master-reference="folge" page-position="rest"/>
            <fo:conditional-page-master-reference pdx:id="folge" master-reference="folge" page-position="last"/>
          </fo:repeatable-page-master-alternatives>
        </fo:page-sequence-master>
      </fo:layout-master-set>
      <fo:declarations>
        <pdx:metadata title="Musterrechnung"/>
      </fo:declarations>
      <fo:page-sequence pdx:id="seiten" id="pdx-sequence" master-reference="seiten" font-family="Helvetica" font-size="9.5pt" line-height="1.3">
        <fo:static-content pdx:id="erste-kopf" flow-name="erste-kopf">
          <fo:block-container pdx:id="briefkopf-firma" absolute-position="absolute" left="0mm" top="0mm" width="165mm">
            <fo:block pdx:id="briefkopf-firma" font-family="Helvetica" font-size="16pt" font-weight="bold" line-height="1.3" color="#1f4e79" text-align="end">
              <fo:inline pdx:id="briefkopf-firma-name">
                <xsl:value-of select="inv:seller/inv:name"/>
              </fo:inline>
            </fo:block>
          </fo:block-container>
          <fo:block-container pdx:id="adressfenster" absolute-position="fixed" left="20mm" top="45mm" width="85mm" height="45mm">
            <fo:block-container pdx:id="absenderzeile" absolute-position="absolute" left="0mm" top="5mm" width="85mm">
              <fo:block pdx:id="absenderzeile" font-family="Helvetica" font-size="7.5pt" line-height="1.3" color="#555555">
                <fo:inline pdx:id="absender-name">
                  <xsl:value-of select="inv:seller/inv:name"/>
                </fo:inline>
                <xsl:text> · </xsl:text>
                <fo:inline pdx:id="absender-strasse">
                  <xsl:value-of select="inv:seller/inv:street"/>
                </fo:inline>
              </fo:block>
            </fo:block-container>
            <xsl:for-each select="inv:buyer[1]">
              <fo:block-container pdx:id="empfaenger" absolute-position="absolute" left="0mm" top="12mm" width="85mm">
                <fo:block pdx:id="empfaenger-name">
                  <xsl:value-of select="inv:name"/>
                </fo:block>
                <fo:block pdx:id="empfaenger-strasse">
                  <xsl:value-of select="inv:street"/>
                </fo:block>
                <fo:block pdx:id="empfaenger-ort">
                  <fo:inline pdx:id="empfaenger-plz">
                    <xsl:value-of select="inv:postcode"/>
                  </fo:inline>
                  <xsl:text> </xsl:text>
                  <fo:inline pdx:id="empfaenger-stadt">
                    <xsl:value-of select="inv:city"/>
                  </fo:inline>
                </fo:block>
              </fo:block-container>
            </xsl:for-each>
          </fo:block-container>
          <fo:block-container pdx:id="rechnungsdaten" absolute-position="fixed" left="125mm" top="50mm" width="60mm">
            <fo:block pdx:id="rechnungsnummer">
              <xsl:text>Rechnungsnummer: </xsl:text>
              <fo:inline pdx:id="rechnungsnummer-wert" font-weight="bold">
                <xsl:value-of select="inv:number"/>
              </fo:inline>
            </fo:block>
            <fo:block pdx:id="rechnungsdatum">
              <xsl:text>Rechnungsdatum: </xsl:text>
              <fo:inline pdx:id="rechnungsdatum-wert">
                <xsl:value-of select="inv:issueDate ! format-date(xs:date(.), '[D01].[M01].[Y0001]', 'de', (), ())"/>
              </fo:inline>
            </fo:block>
          </fo:block-container>
        </fo:static-content>
        <fo:static-content pdx:id="erste-fuss" flow-name="erste-fuss">
          <fo:block-container pdx:id="erste-seitenzahl" absolute-position="absolute" left="0mm" top="4mm" width="165mm">
            <fo:block pdx:id="erste-seitenzahl" font-family="Helvetica" font-size="7.5pt" line-height="1.3" color="#555555" text-align="center">
              <xsl:text>Seite </xsl:text>
              <fo:page-number pdx:id="erste-seitenzahl"/>
              <xsl:text> von </xsl:text>
              <fo:page-number-citation-last pdx:id="erste-seitenzahl" ref-id="pdx-sequence"/>
            </fo:block>
          </fo:block-container>
        </fo:static-content>
        <fo:static-content pdx:id="folge-kopf" flow-name="folge-kopf">
          <fo:block-container pdx:id="folge-titel" absolute-position="absolute" left="0mm" top="0mm" width="165mm">
            <fo:block pdx:id="folge-titel" font-family="Helvetica" font-size="7.5pt" line-height="1.3" color="#555555">
              <fo:inline pdx:id="folge-firma">
                <xsl:value-of select="inv:seller/inv:name"/>
              </fo:inline>
              <xsl:text> · Rechnung </xsl:text>
              <fo:inline pdx:id="folge-nummer">
                <xsl:value-of select="inv:number"/>
              </fo:inline>
            </fo:block>
          </fo:block-container>
        </fo:static-content>
        <fo:static-content pdx:id="folge-fuss" flow-name="folge-fuss">
          <fo:block-container pdx:id="folge-seitenzahl" absolute-position="absolute" left="0mm" top="4mm" width="165mm">
            <fo:block pdx:id="folge-seitenzahl" font-family="Helvetica" font-size="7.5pt" line-height="1.3" color="#555555" text-align="center">
              <xsl:text>Seite </xsl:text>
              <fo:page-number pdx:id="folge-seitenzahl"/>
              <xsl:text> von </xsl:text>
              <fo:page-number-citation-last pdx:id="folge-seitenzahl" ref-id="pdx-sequence"/>
            </fo:block>
          </fo:block-container>
        </fo:static-content>
        <fo:flow pdx:id="inhalt" flow-name="xsl-region-body">
          <fo:block pdx:id="titel" role="H1" font-family="Helvetica" font-size="16pt" font-weight="bold" line-height="1.3" space-after="4mm">
            <xsl:text>Rechnung</xsl:text>
          </fo:block>
          <xsl:variable name="rows-positionen" select="inv:items/inv:item"/>
          <xsl:variable name="context-positionen" select="."/>
          <fo:table pdx:id="positionen" table-layout="fixed" width="100%" border-collapse="collapse" role="Table">
            <fo:table-column pdx:id="sp-pos" column-width="12mm"/>
            <fo:table-column pdx:id="sp-leistung" column-width="proportional-column-width(1)"/>
            <fo:table-column pdx:id="sp-menge" column-width="22mm"/>
            <fo:table-column pdx:id="sp-preis" column-width="25mm"/>
            <fo:table-column pdx:id="sp-betrag" column-width="28mm"/>
            <fo:table-header pdx:id="positionen-kopf">
              <fo:table-row pdx:id="positionen-kopfzeile">
                <fo:table-cell pdx:id="k-pos" font-family="Helvetica" font-size="9.5pt" font-weight="bold" line-height="1.3" color="#ffffff" text-align="end" background-color="#1f4e79" padding="2pt 3pt">
                  <fo:block pdx:id="k-pos-t">
                    <xsl:text>Pos.</xsl:text>
                  </fo:block>
                </fo:table-cell>
                <fo:table-cell pdx:id="k-leistung" font-family="Helvetica" font-size="9.5pt" font-weight="bold" line-height="1.3" color="#ffffff" background-color="#1f4e79" padding="2pt 3pt">
                  <fo:block pdx:id="k-leistung-t">
                    <xsl:text>Leistung</xsl:text>
                  </fo:block>
                </fo:table-cell>
                <fo:table-cell pdx:id="k-menge" font-family="Helvetica" font-size="9.5pt" font-weight="bold" line-height="1.3" color="#ffffff" text-align="end" background-color="#1f4e79" padding="2pt 3pt">
                  <fo:block pdx:id="k-menge-t">
                    <xsl:text>Menge</xsl:text>
                  </fo:block>
                </fo:table-cell>
                <fo:table-cell pdx:id="k-preis" font-family="Helvetica" font-size="9.5pt" font-weight="bold" line-height="1.3" color="#ffffff" text-align="end" background-color="#1f4e79" padding="2pt 3pt">
                  <fo:block pdx:id="k-preis-t">
                    <xsl:text>Einzelpreis</xsl:text>
                  </fo:block>
                </fo:table-cell>
                <fo:table-cell pdx:id="k-betrag" font-family="Helvetica" font-size="9.5pt" font-weight="bold" line-height="1.3" color="#ffffff" text-align="end" background-color="#1f4e79" padding="2pt 3pt">
                  <fo:block pdx:id="k-betrag-t">
                    <xsl:text>Betrag</xsl:text>
                  </fo:block>
                </fo:table-cell>
              </fo:table-row>
              <fo:table-row pdx:id="uebertragszeile" pdx:artifact="true">
                <fo:table-cell pdx:id="u-label" number-columns-spanned="4" font-family="Helvetica" font-size="9.5pt" line-height="1.3" text-align="end" padding="2pt 3pt" border-bottom="0.25pt solid #cccccc">
                  <fo:block pdx:id="u-label">
                    <fo:retrieve-table-marker pdx:id="u-label" retrieve-class-name="u-label" retrieve-position-within-table="first-starting" retrieve-boundary-within-table="table-fragment"/>
                  </fo:block>
                </fo:table-cell>
                <fo:table-cell pdx:id="u-wert" font-family="Helvetica" font-size="9.5pt" line-height="1.3" text-align="end" padding="2pt 3pt" border-bottom="0.25pt solid #cccccc">
                  <fo:block pdx:id="u-carry">
                    <fo:retrieve-table-marker pdx:id="u-carry" retrieve-class-name="u-carry" retrieve-position-within-table="first-starting" retrieve-boundary-within-table="table-fragment"/>
                  </fo:block>
                </fo:table-cell>
              </fo:table-row>
            </fo:table-header>
            <fo:table-footer pdx:id="positionen-fuss">
              <fo:table-row pdx:id="zwischensummenzeile" pdx:artifact="true">
                <fo:table-cell pdx:id="zs-label" number-columns-spanned="4" font-family="Helvetica" font-size="9.5pt" font-weight="bold" line-height="1.3" text-align="end" padding="2pt 3pt" border-bottom="0.25pt solid #cccccc">
                  <fo:block pdx:id="zs-label-t">
                    <xsl:text>Zwischensumme</xsl:text>
                  </fo:block>
                </fo:table-cell>
                <fo:table-cell pdx:id="zs-wert" font-family="Helvetica" font-size="9.5pt" font-weight="bold" line-height="1.3" text-align="end" padding="2pt 3pt" border-bottom="0.25pt solid #cccccc">
                  <fo:block pdx:id="zs-sub">
                    <fo:retrieve-table-marker pdx:id="zs-sub" retrieve-class-name="zs-sub" retrieve-position-within-table="last-ending" retrieve-boundary-within-table="table-fragment"/>
                  </fo:block>
                </fo:table-cell>
              </fo:table-row>
            </fo:table-footer>
            <fo:table-body pdx:id="positionen-zeilen">
              <xsl:if test="empty($rows-positionen)">
                <fo:table-row pdx:id="positionen-zeilen">
                  <fo:table-cell pdx:id="positionen-zeilen" number-columns-spanned="5">
                    <fo:block pdx:id="positionen-zeilen"/>
                  </fo:table-cell>
                </fo:table-row>
              </xsl:if>
              <xsl:iterate select="$rows-positionen">
                <xsl:param name="index-positionen" select="1" as="xs:integer"/>
                <xsl:param name="sum-positionen-1" select="0" as="xs:decimal"/>
                <xsl:variable name="next-positionen-1" select="$sum-positionen-1 + sum(xs:decimal(inv:amount))" as="xs:decimal"/>
                <fo:table-row pdx:id="position">
                  <fo:table-cell pdx:id="z-pos" font-family="Helvetica" font-size="9.5pt" line-height="1.3" text-align="end" padding="2pt 3pt" border-bottom="0.25pt solid #cccccc">
                    <fo:marker marker-class-name="u-carry">
                      <xsl:if test="$index-positionen gt 1">
                        <xsl:value-of select="format-number(xs:decimal($sum-positionen-1), '#.##0,00', 'fmt-de')"/>
                      </xsl:if>
                    </fo:marker>
                    <fo:marker marker-class-name="u-label">
                      <xsl:if test="$index-positionen gt 1">
                        <xsl:for-each select="$context-positionen">
                          <fo:inline pdx:id="u-label-t">
                            <xsl:text>Übertrag</xsl:text>
                          </fo:inline>
                        </xsl:for-each>
                      </xsl:if>
                    </fo:marker>
                    <fo:marker marker-class-name="zs-sub">
                      <xsl:value-of select="format-number(xs:decimal($next-positionen-1), '#.##0,00', 'fmt-de')"/>
                    </fo:marker>
                    <fo:block pdx:id="z-pos-f">
                      <xsl:value-of select="inv:position"/>
                    </fo:block>
                  </fo:table-cell>
                  <fo:table-cell pdx:id="z-leistung" font-family="Helvetica" font-size="9.5pt" line-height="1.3" padding="2pt 3pt" border-bottom="0.25pt solid #cccccc">
                    <fo:block pdx:id="z-leistung-f">
                      <xsl:value-of select="inv:description"/>
                    </fo:block>
                  </fo:table-cell>
                  <fo:table-cell pdx:id="z-menge" font-family="Helvetica" font-size="9.5pt" line-height="1.3" text-align="end" padding="2pt 3pt" border-bottom="0.25pt solid #cccccc">
                    <fo:block pdx:id="z-menge-f">
                      <xsl:value-of select="inv:quantity ! format-number(xs:decimal(.), '#.##0,###', 'fmt-de')"/>
                    </fo:block>
                  </fo:table-cell>
                  <fo:table-cell pdx:id="z-preis" font-family="Helvetica" font-size="9.5pt" line-height="1.3" text-align="end" padding="2pt 3pt" border-bottom="0.25pt solid #cccccc">
                    <fo:block pdx:id="z-preis-f">
                      <xsl:value-of select="inv:unitPrice ! format-number(xs:decimal(.), '#.##0,00', 'fmt-de')"/>
                    </fo:block>
                  </fo:table-cell>
                  <fo:table-cell pdx:id="z-betrag" font-family="Helvetica" font-size="9.5pt" line-height="1.3" text-align="end" padding="2pt 3pt" border-bottom="0.25pt solid #cccccc">
                    <fo:block pdx:id="z-betrag-f">
                      <xsl:value-of select="inv:amount ! format-number(xs:decimal(.), '#.##0,00', 'fmt-de')"/>
                    </fo:block>
                  </fo:table-cell>
                </fo:table-row>
                <xsl:next-iteration>
                  <xsl:with-param name="index-positionen" select="$index-positionen + 1"/>
                  <xsl:with-param name="sum-positionen-1" select="$next-positionen-1"/>
                </xsl:next-iteration>
              </xsl:iterate>
            </fo:table-body>
          </fo:table>
          <fo:block pdx:id="summen" keep-together.within-page="always" space-before="4mm">
            <fo:block pdx:id="netto">
              <xsl:text>Nettobetrag: </xsl:text>
              <fo:inline pdx:id="netto-wert">
                <xsl:value-of select="inv:totals/inv:net ! format-number(xs:decimal(.), '#.##0,00', 'fmt-de')"/>
              </fo:inline>
              <xsl:text> €</xsl:text>
            </fo:block>
            <fo:block pdx:id="steuer">
              <xsl:text>Umsatzsteuer: </xsl:text>
              <fo:inline pdx:id="steuer-wert">
                <xsl:value-of select="inv:totals/inv:tax ! format-number(xs:decimal(.), '#.##0,00', 'fmt-de')"/>
              </fo:inline>
              <xsl:text> €</xsl:text>
            </fo:block>
            <fo:block pdx:id="brutto" font-weight="bold">
              <xsl:text>Gesamtbetrag: </xsl:text>
              <fo:inline pdx:id="brutto-wert">
                <xsl:value-of select="inv:totals/inv:gross ! format-number(xs:decimal(.), '#.##0,00', 'fmt-de')"/>
              </fo:inline>
              <xsl:text> €</xsl:text>
            </fo:block>
            <fo:block pdx:id="girocode">
              <fo:instream-foreign-object pdx:id="girocode" content-width="30mm">
                <pdx:barcode type="girocode">
                  <xsl:attribute name="name" select="inv:seller/inv:name"/>
                  <xsl:attribute name="iban" select="inv:seller/inv:iban"/>
                  <xsl:attribute name="amount" select="inv:totals/inv:gross"/>
                  <xsl:attribute name="text" select="inv:number"/>
                </pdx:barcode>
              </fo:instream-foreign-object>
            </fo:block>
          </fo:block>
        </fo:flow>
      </fo:page-sequence>
    </fo:root>
  </xsl:template>
</xsl:stylesheet>
