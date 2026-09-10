package com.edgewatcher.di;

import android.content.Context;
import com.edgewatcher.domain.port.CredentialStore;
import dagger.internal.DaggerGenerated;
import dagger.internal.Factory;
import dagger.internal.Preconditions;
import dagger.internal.Provider;
import dagger.internal.QualifierMetadata;
import dagger.internal.ScopeMetadata;
import javax.annotation.processing.Generated;

@ScopeMetadata("javax.inject.Singleton")
@QualifierMetadata("dagger.hilt.android.qualifiers.ApplicationContext")
@DaggerGenerated
@Generated(
    value = "dagger.internal.codegen.ComponentProcessor",
    comments = "https://dagger.dev"
)
@SuppressWarnings({
    "unchecked",
    "rawtypes",
    "KotlinInternal",
    "KotlinInternalInJava",
    "cast",
    "deprecation",
    "nullness:initialization.field.uninitialized"
})
public final class AppModule_CredentialStoreFactory implements Factory<CredentialStore> {
  private final Provider<Context> contextProvider;

  private AppModule_CredentialStoreFactory(Provider<Context> contextProvider) {
    this.contextProvider = contextProvider;
  }

  @Override
  public CredentialStore get() {
    return credentialStore(contextProvider.get());
  }

  public static AppModule_CredentialStoreFactory create(Provider<Context> contextProvider) {
    return new AppModule_CredentialStoreFactory(contextProvider);
  }

  public static CredentialStore credentialStore(Context context) {
    return Preconditions.checkNotNullFromProvides(AppModule.INSTANCE.credentialStore(context));
  }
}
