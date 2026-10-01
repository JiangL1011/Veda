!macro veda.associateFolder
    WriteRegStr SHELL_CONTEXT "Software\Classes\Directory\shell\Veda" "" "使用 Veda 打开"
    WriteRegStr SHELL_CONTEXT "Software\Classes\Directory\shell\Veda" "Icon" "$INSTDIR\${PRODUCT_EXECUTABLE},0"
    WriteRegStr SHELL_CONTEXT "Software\Classes\Directory\shell\Veda\command" "" "$\"$INSTDIR\${PRODUCT_EXECUTABLE}$\" $\"%1$\""
!macroend

!macro veda.unassociateFolder
    DeleteRegKey SHELL_CONTEXT "Software\Classes\Directory\shell\Veda"
!macroend
